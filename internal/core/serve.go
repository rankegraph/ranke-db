// package: core / orchestration
// type:    logic
// job:     put the library's bytes on the wire — a switch on kind, plus its separator
// limits:  transport only; it encodes no claim and shapes no result (-> github.com/rankegraph/ranke-go)
//
// The engine answers a query already shaped and serialised to every output axis, and
// tags each result with the one field carrying its payload. So serving is a switch and
// a separator: pull a result, write that field, frame it. Re-encoding here would put a
// claim's canonical form in the server's hands, and the id is a signature over exactly
// those bytes — the server would then be deciding identity.
//
// The streams are lazy by construction. Each pulls one result at a time and writes it
// straight to the response, so a million-row query and a multi-gigabyte blob both cost
// one record of memory.
package core

import (
	"encoding/json"
	"fmt"
	"io"

	ranke "github.com/rankegraph/ranke-go"
)

// Media types the streams below produce.
const (
	mediaJSON    = "application/json"
	mediaJSONSeq = "application/json-seq" // RFC 7464: each record RS-prefixed, LF-terminated
	mediaCBOR    = "application/cbor"
	mediaCBORSeq = "application/cbor-seq" // RFC 8742: records concatenated
	mediaBlob    = "application/octet-stream"
)

// recordSeparator opens every record of an RFC 7464 sequence.
const recordSeparator = 0x1e

// --- a query's result set -------------------------------------------------

// queryStream serves a ResultStream: one record per result, then the execution report
// if the query asked for one.
type queryStream struct {
	results ranke.ResultStream
	seq     sequenceFraming
}

// newQueryStream frames a result stream per the encoding the query asked for.
func newQueryStream(results ranke.ResultStream, enc ranke.ResultEncoding) *queryStream {
	return &queryStream{results: results, seq: framingFor(enc)}
}

// ContentType is the sequence media type matching the query's encoding.
func (s *queryStream) ContentType() string { return s.seq.mediaType }

// WriteTo pulls each result and writes the field its Kind names, framed. A report is
// written last, so a reader that knows the framing can always tell it from a result.
func (s *queryStream) WriteTo(w io.Writer) (int64, error) {
	var n int64
	for s.results.Next() {
		// The report is the stream's own final element (`R-QSTREAM`), and is written
		// below once the results are out. Stepping over it here is what keeps that
		// from asking a report for the payload field a result carries — which it has
		// none of, so the write failed and took the report off the wire with it.
		if s.results.Result().Kind == ranke.KindReport {
			continue
		}
		written, err := s.writeResult(w, s.results.Result())
		n += written
		if err != nil {
			return n, err
		}
	}
	if err := s.results.Err(); err != nil {
		return n, mapLibError(err)
	}
	if report := s.results.Report(); report != nil {
		written, err := s.seq.writeValue(w, report)
		n += written
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// writeResult writes the one payload field a result's Kind names.
func (s *queryStream) writeResult(w io.Writer, r ranke.QueryResult) (int64, error) {
	switch r.Kind {
	case ranke.KindClaimEncoded:
		return s.seq.writeRecord(w, r.ClaimEncoded)
	case ranke.KindPathEncoded:
		var n int64
		for _, claim := range r.PathEncoded {
			written, err := s.seq.writeRecord(w, claim)
			n += written
			if err != nil {
				return n, err
			}
		}
		return n, nil
	case ranke.KindClaimEnvelope:
		// Envelope bytes ride the same fields as an encoded claim (ranke-go's
		// encodeEach fills ClaimEncoded/PathEncoded regardless of kind) — only the
		// Kind tag tells a reader it holds the stored envelope, not an assembled
		// record (R-QCANON, R-QSTREAM).
		return s.seq.writeRecord(w, r.ClaimEncoded)
	case ranke.KindPathEnvelope:
		var n int64
		for _, claim := range r.PathEncoded {
			written, err := s.seq.writeRecord(w, claim)
			n += written
			if err != nil {
				return n, err
			}
		}
		return n, nil
	case ranke.KindClaimId:
		return s.seq.writeValue(w, r.ClaimId.String())
	case ranke.KindPathId:
		ids := make([]string, 0, len(r.PathId))
		for _, id := range r.PathId {
			ids = append(ids, id.String())
		}
		return s.seq.writeValue(w, ids)
	default:
		// The native kinds carry Go objects, which only an encoding could put on the
		// wire — and that encoding is the engine's. dispatch pins an explicit encoding
		// so the engine always serialises, making this unreachable rather than a
		// fallback worth writing.
		return 0, fmt.Errorf("query returned %q, which carries no serialized payload", r.Kind)
	}
}

// Close releases the underlying result stream.
func (s *queryStream) Close() error { return s.results.Close() }

// --- framing --------------------------------------------------------------

// sequenceFraming is how one media type delimits the records of a sequence.
type sequenceFraming struct {
	mediaType string
	// prefix and suffix bracket each record; either may be empty.
	prefix, suffix []byte
}

// framingFor pairs an encoding with the sequence media type that frames it.
func framingFor(enc ranke.ResultEncoding) sequenceFraming {
	if enc == ranke.ResultCBOR {
		return sequenceFraming{mediaType: mediaCBORSeq}
	}
	return sequenceFraming{mediaType: mediaJSONSeq, prefix: []byte{recordSeparator}, suffix: []byte("\n")}
}

// writeRecord writes one record with its delimiters.
func (f sequenceFraming) writeRecord(w io.Writer, payload []byte) (int64, error) {
	var n int64
	for _, part := range [][]byte{f.prefix, payload, f.suffix} {
		if len(part) == 0 {
			continue
		}
		written, err := w.Write(part)
		n += int64(written)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// writeValue writes v as one record in the framing's OWN encoding. It serves the
// payloads that are not claim bytes — an id, a route of ids, the execution report —
// none of which the engine serialises and none of which an id is computed over.
//
// The encoding follows the framing rather than defaulting to JSON: a JSON record in a
// CBOR sequence mis-decodes rather than failing, since a leading '"' is a valid CBOR
// negative integer. §Output has both encodings carry the same information, which
// means each carries it in its own form.
func (f sequenceFraming) writeValue(w io.Writer, v any) (int64, error) {
	marshal := json.Marshal
	if f.mediaType == mediaCBORSeq {
		// ranke-go's deterministic encoder, so one encoder serves every record in the
		// sequence and a map's keys order the way a claim's do.
		marshal = ranke.MarshalCBOR
	}
	payload, err := marshal(v)
	if err != nil {
		return 0, err
	}
	return f.writeRecord(w, payload)
}

// --- single values and blobs ---------------------------------------------

// jsonStream serves one JSON value: a branch head, a layer list, a health report.
type jsonStream struct {
	value any
}

// ContentType is application/json.
func (s *jsonStream) ContentType() string { return mediaJSON }

// WriteTo encodes the value.
func (s *jsonStream) WriteTo(w io.Writer) (int64, error) {
	payload, err := json.Marshal(s.value)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(payload)
	return int64(n), err
}

// Close is a no-op: a marshalled value holds nothing.
func (s *jsonStream) Close() error { return nil }

// bytesStream serves bytes the library already produced — one claim's stored CBOR.
type bytesStream struct {
	payload     []byte
	contentType string
}

// ContentType is the media type the bytes were declared with.
func (s *bytesStream) ContentType() string { return s.contentType }

// WriteTo copies the bytes through unaltered.
func (s *bytesStream) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(s.payload)
	return int64(n), err
}

// Close is a no-op: the bytes are already in hand.
func (s *bytesStream) Close() error { return nil }

// blobStream serves a claim's content as raw bytes, copied incrementally so a large
// blob is never held whole.
type blobStream struct {
	content io.ReadCloser
}

// ContentType is application/octet-stream: content is opaque bytes.
func (s *blobStream) ContentType() string { return mediaBlob }

// WriteTo copies the content through.
func (s *blobStream) WriteTo(w io.Writer) (int64, error) { return io.Copy(w, s.content) }

// Close releases the underlying reader.
func (s *blobStream) Close() error { return s.content.Close() }
