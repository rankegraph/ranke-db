package storage

import (
	"bytes"
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/tools/podman"
)

// TestParseSize covers the human-readable size suffixes the maxContentSize field
// accepts.
func TestParseSize(t *testing.T) {
	cases := map[string]uint64{
		"":      0,
		"512":   512,
		"8kb":   8 << 10,
		"2 MB":  2 << 20,
		"1gb":   1 << 30,
		"4096b": 4096,
	}
	for in, want := range cases {
		got, err := parseSize(in)
		if err != nil {
			t.Errorf("parseSize(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := parseSize("not-a-size"); err == nil {
		t.Error("parseSize(\"not-a-size\") = nil error, want error")
	}
}

// TestBuildRedis covers the redis wiring: client construction is local (no
// dial), so this needs no reachable redis instance.
func TestBuildRedis(t *testing.T) {
	if _, err := New(context.Background(), scope.Literal(map[string]string{"type": "redis"})); err == nil {
		t.Error("New(redis without addr) = nil error, want error")
	}
	if _, err := New(context.Background(), scope.Literal(map[string]string{
		"type": "redis", "addr": "127.0.0.1:6379", "db": "not-a-number",
	})); err == nil {
		t.Error("New(redis, db=not-a-number) = nil error, want error")
	}
	u, err := New(context.Background(), scope.Literal(map[string]string{
		"type": "redis", "addr": "127.0.0.1:6379", "password": "secret", "db": "3",
	}))
	if err != nil {
		t.Fatalf("New(redis): %v", err)
	}
	if u == nil {
		t.Fatal("New(redis) returned a nil Universe")
	}
}

// TestBuildS3 covers the s3 wiring: New only fails for a nil client or empty
// bucket, so an unreachable endpoint still builds — probeCaps swallows a probe
// failure into all-false capabilities rather than erroring construction.
func TestBuildS3(t *testing.T) {
	required := map[string]string{
		"type": "s3", "bucket": "b", "region": "us-east-1",
		"accessKeyId": "ak", "secretAccessKey": "sk",
	}
	for _, missing := range []string{"bucket", "region", "accessKeyId", "secretAccessKey"} {
		partial := map[string]string{}
		for k, v := range required {
			if k != missing {
				partial[k] = v
			}
		}
		if _, err := New(context.Background(), scope.Literal(partial)); err == nil {
			t.Errorf("New(s3 without %s) = nil error, want error", missing)
		}
	}
	full := map[string]string{}
	for k, v := range required {
		full[k] = v
	}
	full["endpoint"] = "http://127.0.0.1:1" // refused instantly, no live service needed
	full["usePathStyle"] = "true"
	u, err := New(context.Background(), scope.Literal(full))
	if err != nil {
		t.Fatalf("New(s3): %v", err)
	}
	if u == nil {
		t.Fatal("New(s3) returned a nil Universe")
	}
}

// TestBuildNeo4j covers the neo4j wiring: NewDriverWithContext validates the
// URI scheme and pools connections lazily, so this needs no reachable instance.
func TestBuildNeo4j(t *testing.T) {
	if _, err := New(context.Background(), scope.Literal(map[string]string{"type": "neo4j"})); err == nil {
		t.Error("New(neo4j without uri) = nil error, want error")
	}
	u, err := New(context.Background(), scope.Literal(map[string]string{
		"type": "neo4j", "uri": "bolt://127.0.0.1:7687",
	}))
	if err != nil {
		t.Fatalf("New(neo4j, no auth): %v", err)
	}
	if u == nil {
		t.Fatal("New(neo4j, no auth) returned a nil Universe")
	}
	u, err = New(context.Background(), scope.Literal(map[string]string{
		"type": "neo4j", "uri": "bolt://127.0.0.1:7687",
		"username": "neo4j", "password": "secret", "database": "ranke",
	}))
	if err != nil {
		t.Fatalf("New(neo4j): %v", err)
	}
	if u == nil {
		t.Fatal("New(neo4j) returned a nil Universe")
	}
}

// azuriteKey is Azurite's published development account key — the well-known value every
// emulator uses, here only because NewSharedKeyCredential decodes it as base64.
const azuriteKey = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

// deadBlobEndpoint is a port nothing listens on: the capability probe runs at construction
// and swallows what it cannot reach, so the wiring is exercised with no live service.
const deadBlobEndpoint = "http://127.0.0.1:1/devstoreaccount1"

// TestBuildAzure covers what the Azure Blob leaf refuses without reaching a service: the
// container is required, and so is a way to reach the account.
func TestBuildAzure(t *testing.T) {
	ctx := context.Background()

	for name, cfg := range map[string]map[string]string{
		"no container":                    {"type": "azure", "url": deadBlobEndpoint},
		"no url and no connection string": {"type": "azure", "container": "ranke"},
		"a key without its account": {
			"type": "azure", "container": "ranke", "url": deadBlobEndpoint, "accountKey": azuriteKey,
		},
		"concurrency that is not a count": {
			"type": "azure", "container": "ranke", "url": deadBlobEndpoint,
			"accountName": "devstoreaccount1", "accountKey": azuriteKey, "concurrency": "several",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(ctx, scope.Literal(cfg)); err == nil {
				t.Error("want an error")
			}
		})
	}
}

// TestAzureStoresThroughTheUniverse drives the leaf against Azurite, the emulator ranke-go
// tests the store itself against: the config builds a client, the container answers, and
// content put through the Universe reads back. It skips without podman.
func TestAzureStoresThroughTheUniverse(t *testing.T) {
	ctx := context.Background()
	addr, teardown := podman.Run(t, podman.Spec{
		Image: "mcr.microsoft.com/azure-storage/azurite",
		Port:  10000,
		// The SDK speaks a newer service version than the emulator knows; the check is
		// the emulator's own opt-out, and the operations here are long settled.
		Args: []string{"azurite-blob", "--blobHost", "0.0.0.0", "--blobPort", "10000", "--skipApiVersionCheck"},
	})
	t.Cleanup(teardown)

	conn := "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=" + azuriteKey +
		";BlobEndpoint=http://" + addr + "/devstoreaccount1;"
	client, err := azblob.NewClientFromConnectionString(conn, nil)
	if err != nil {
		t.Fatalf("azurite client: %v", err)
	}
	if _, err := client.CreateContainer(ctx, "ranke", nil); err != nil {
		t.Fatalf("create the container: %v", err)
	}
	t.Log("▸ Azurite is serving a container named ranke")

	u, err := New(ctx, scope.Literal(map[string]string{
		"type": "azure", "container": "ranke", "connectionString": conn, "concurrency": "4",
	}))
	if err != nil {
		t.Fatalf("New(azure): %v", err)
	}

	content := []byte("ranke-db azure blob leaf")
	hash, err := ranke.HashContent(content)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := u.PutContents(ctx, []ranke.ContentBlob{{Hash: hash, Content: content}}); err != nil {
		t.Fatalf("PutContents: %v", err)
	}
	got, err := u.GetContents(ctx, []ranke.ContentRef{{Hash: hash, ContentSize: uint64(len(content))}})
	if err != nil {
		t.Fatalf("GetContents: %v", err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], content) {
		t.Fatalf("GetContents returned %d blob(s), want the one put back intact", len(got))
	}
	t.Log("▸ content written through the Universe read back from the container — OK")
}
