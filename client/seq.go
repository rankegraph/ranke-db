// package: client / transport
// type:    io
// job:     the two framings a read arrives in — RFC 7464 JSON text sequences and RFC 8742 CBOR
// sequences — split into records, and each record read back as the result it carries
// limits:  the response side only; the contribution stream is the format's and stays there
// (-> ranke-go codec_wire)
//
// Framing is the endpoint contract's, these being media types a response negotiates, and
// ranke-go ships the encode direction alone.
package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/fxamacker/cbor/v2"

	"github.com/rankegraph/ranke-go"
)

// Media types the read endpoints answer with, each mirroring output.encoding, which the
// server pins before execution so the response always declares what it wrote.
const (
	MediaJSONSeq = "application/json-seq"
	MediaCBORSeq = "application/cbor-seq"
)

// recordSeparator is RFC 7464's 0x1e, which opens every record in a JSON text sequence.
const recordSeparator = 0x1e

// splitBody picks the framing from the response's media type, never from what the query
// asked for, so a caller never has to agree with the server twice.
func splitBody(resp *http.Response) ([][]byte, ranke.ResultEncoding, error) {
	ct, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		ct = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}
	switch ct {
	case MediaJSONSeq:
		records, err := splitJSONSeq(resp.Body)
		return records, ranke.ResultJSON, err
	case MediaCBORSeq:
		records, err := splitCBORSeq(resp.Body)
		return records, ranke.ResultCBOR, err
	default:
		return nil, "", ranke.WithDetail(ErrUnknownFraming, ct)
	}
}

// splitJSONSeq returns each record of an RFC 7464 stream, stripped of its separator and
// trailing newline. A record may span lines, so the separator alone delimits.
func splitJSONSeq(r io.Reader) ([][]byte, error) {
	br := bufio.NewReader(r)
	var out [][]byte
	for {
		chunk, err := br.ReadBytes(recordSeparator)
		trimmed := bytes.TrimSpace(bytes.TrimSuffix(chunk, []byte{recordSeparator}))
		if len(trimmed) > 0 {
			out = append(out, trimmed)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
	}
}

// splitCBORSeq returns each record of an RFC 8742 stream, which the endpoint writes as
// bare concatenation: an item is self-delimiting, so one ends where the next begins.
func splitCBORSeq(r io.Reader) ([][]byte, error) {
	dec := cbor.NewDecoder(r)
	var out [][]byte
	for {
		var raw cbor.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
}

// decodeRecord reads one record as the QueryResult ranke-go describes a result with, so
// Query switches on Kind once, as an in-process reader does. A serialized claim comes
// back as bytes under KindClaimEncoded, ranke-go exporting no reader for that form.
//
// The reading mirrors ranke-ts's decodeResultRecord: payload inspection, which
// `R-QSTREAM` would rather forbid and the wire carries no tag to replace. Mirroring the
// tested reference is what lets the explorer and this move together when the tag lands.
func decodeRecord(raw []byte, enc ranke.ResultEncoding) (ranke.QueryResult, error) {
	if len(raw) == 0 {
		return ranke.QueryResult{}, ranke.WithDetail(ErrUnknownFraming, "an empty record carries nothing")
	}
	if enc == ranke.ResultCBOR {
		return decodeCBORRecord(raw)
	}
	return decodeJSONRecord(raw)
}

// decodeJSONRecord reads a record of a json-seq response by its JSON shape.
func decodeJSONRecord(raw []byte) (ranke.QueryResult, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ranke.QueryResult{}, ranke.Wrap(ErrUnknownFraming, err)
	}
	switch v := value.(type) {
	case string:
		id, err := ranke.ParseId(v)
		if err != nil {
			return ranke.QueryResult{}, err
		}
		return ranke.QueryResult{Kind: ranke.KindClaimId, ClaimId: id}, nil
	case []any:
		ids := make([]ranke.Id, 0, len(v))
		for _, entry := range v {
			s, ok := entry.(string)
			if !ok {
				return ranke.QueryResult{}, ranke.WithDetail(ErrUnknownFraming, "a route of ids holds strings")
			}
			id, err := ranke.ParseId(s)
			if err != nil {
				return ranke.QueryResult{}, err
			}
			ids = append(ids, id)
		}
		return ranke.QueryResult{Kind: ranke.KindPathId, PathId: ids}, nil
	case map[string]any:
		_, typed := v["type"]
		_, dated := v["created_at"]
		if typed && dated {
			return ranke.QueryResult{Kind: ranke.KindClaimEncoded, ClaimEncoded: raw}, nil
		}
		var report ranke.QueryReport
		if err := json.Unmarshal(raw, &report); err != nil {
			return ranke.QueryResult{}, ranke.Wrap(ErrUnknownFraming, err)
		}
		return ranke.QueryResult{Kind: ranke.KindReport, Report: &report}, nil
	default:
		return ranke.QueryResult{}, ranke.WithDetail(ErrUnknownFraming, "a result record is a string, a list or an object")
	}
}

// CBOR major types, read from a record's first byte: every payload in a cbor sequence is
// CBOR, so the kinds discriminate as they do under json.
const (
	majorText  = 3
	majorArray = 4
	majorMap   = 5
	majorTag   = 6
)

// decodeCBORRecord reads a record of a cbor-seq response by its CBOR major type.
func decodeCBORRecord(raw []byte) (ranke.QueryResult, error) {
	switch raw[0] >> 5 {
	case majorText:
		var s string
		if err := cbor.Unmarshal(raw, &s); err != nil {
			return ranke.QueryResult{}, ranke.Wrap(ErrUnknownFraming, err)
		}
		id, err := ranke.ParseId(s)
		if err != nil {
			return ranke.QueryResult{}, err
		}
		return ranke.QueryResult{Kind: ranke.KindClaimId, ClaimId: id}, nil
	case majorArray:
		var names []string
		if err := cbor.Unmarshal(raw, &names); err != nil {
			return ranke.QueryResult{}, ranke.Wrap(ErrUnknownFraming, err)
		}
		ids := make([]ranke.Id, 0, len(names))
		for _, name := range names {
			id, err := ranke.ParseId(name)
			if err != nil {
				return ranke.QueryResult{}, err
			}
			ids = append(ids, id)
		}
		return ranke.QueryResult{Kind: ranke.KindPathId, PathId: ids}, nil
	case majorMap:
		// A claim's record keys are integers (`V-SER`), a report's the field names.
		key, ok := firstMapKeyMajor(raw)
		if !ok {
			return ranke.QueryResult{}, ranke.WithDetail(ErrUnknownFraming, "a map record ends before its first key")
		}
		if key == majorText {
			var report ranke.QueryReport
			if err := cbor.Unmarshal(raw, &report); err != nil {
				return ranke.QueryResult{}, ranke.Wrap(ErrUnknownFraming, err)
			}
			return ranke.QueryResult{Kind: ranke.KindReport, Report: &report}, nil
		}
		return ranke.QueryResult{Kind: ranke.KindClaimEncoded, ClaimEncoded: raw}, nil
	case majorTag:
		// The stored record, tagged apart from a serialized claim (`R-QSTREAM`). Its bytes
		// hash to the id it answers for, which is why one asks for this form (`R-QCANON`).
		claim, err := decodeStored(raw)
		if err != nil {
			return ranke.QueryResult{}, err
		}
		return ranke.QueryResult{Kind: ranke.KindClaimEnvelope, ClaimEncoded: raw, ClaimNative: claim}, nil
	default:
		return ranke.QueryResult{}, ranke.WithDetail(ErrUnknownFraming,
			"a result record is a text string, a list, a map or a tag")
	}
}

// firstMapKeyMajor is the major type of a CBOR map's first key, past the count bytes.
func firstMapKeyMajor(raw []byte) (byte, bool) {
	header := 1
	switch info := raw[0] & 0x1f; {
	case info < 24, info == 31: // count inline, or indefinite length
	case info == 24:
		header = 2
	case info == 25:
		header = 3
	case info == 26:
		header = 5
	case info == 27:
		header = 9
	default:
		return 0, false
	}
	if len(raw) <= header {
		return 0, false
	}
	return raw[header] >> 5, true
}

// decodeStored derives the id a stored record must carry: id(v) is the hash of exactly
// these bytes, so nothing has to travel beside it.
func decodeStored(raw []byte) (ranke.Claim, error) {
	id, err := ranke.HashContent(raw)
	if err != nil {
		return nil, err
	}
	return ranke.DecodeClaim(id, raw)
}
