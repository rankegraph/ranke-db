// package: storage / composition
// type:    factory
// job:     build one ranke.Universe from a storage section — a leaf, or a composite
// limits:  wiring only; the persistence logic is ranke-go's adapters (-> github.com/rankegraph/ranke-go)
//
// A composed stack or partition is itself a Universe, so the whole store is one
// recursive descriptor: a "type" selects a leaf or a composite, and composites recurse.
//
// A layer's write role and content cap are the backend's, reported as
// ranke.Capabilities, so this package only fails a declaration that disagrees.
package storage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	neo4jdriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	goredis "github.com/redis/go-redis/v9"

	"github.com/rankegraph/ranke-go"
	storageazure "github.com/rankegraph/ranke-go/adapter/storage/azure"
	"github.com/rankegraph/ranke-go/adapter/storage/fs"
	"github.com/rankegraph/ranke-go/adapter/storage/mem"
	"github.com/rankegraph/ranke-go/adapter/storage/minimal"
	storageneo4j "github.com/rankegraph/ranke-go/adapter/storage/neo4j"
	"github.com/rankegraph/ranke-go/adapter/storage/partition"
	storageredis "github.com/rankegraph/ranke-go/adapter/storage/redis"
	storages3 "github.com/rankegraph/ranke-go/adapter/storage/s3"
	"github.com/rankegraph/ranke-go/adapter/storage/sqlite"
	"github.com/rankegraph/ranke-go/adapter/storage/stack"

	"github.com/rankegraph/ranke-db/config/scope"
)

// Storage is the storage port's product: a ranke.Universe. A composed stack or partition
// implements Universe too, so the whole store presents as this one type.
type Storage = ranke.Universe

// New builds the storage universe described by sec (see the package doc for the
// descriptor shape).
func New(ctx context.Context, sec scope.Section) (Storage, error) {
	return build(ctx, sec)
}

// Layer is what introspection may report of a configured layer: a name and a type.
type Layer struct {
	Name string
	Type string
}

// Describe reports the configured layers top to bottom, touching no backend. A layer
// takes its optional "name" key, else its type.
func Describe(ctx context.Context, sec scope.Section) ([]Layer, error) {
	t, err := typeOf(ctx, sec)
	if err != nil {
		return nil, err
	}
	descriptors := []scope.Section{sec}
	switch t {
	case "stack":
		descriptors = sec.GetArray("layers")
	case "partition":
		descriptors = sec.GetArray("shards")
	}

	layers := make([]Layer, 0, len(descriptors))
	for _, d := range descriptors {
		lt, err := typeOf(ctx, d)
		if err != nil {
			return nil, err
		}
		name := lt
		if d.HasValue("name") {
			if name, err = d.Get(ctx, "name"); err != nil {
				return nil, fmt.Errorf("storage: layer name: %w", err)
			}
		}
		layers = append(layers, Layer{Name: name, Type: lt})
	}
	return distinct(layers), nil
}

// distinct suffixes a repeated name with its position, so every name identifies one layer.
func distinct(layers []Layer) []Layer {
	seen := make(map[string]int, len(layers))
	for _, l := range layers {
		seen[l.Name]++
	}
	for i, l := range layers {
		if seen[l.Name] > 1 {
			layers[i].Name = fmt.Sprintf("%s-%d", l.Name, i)
		}
	}
	return layers
}

// build dispatches one descriptor to a leaf backend or a composite, recursing.
func build(ctx context.Context, sec scope.Section) (ranke.Universe, error) {
	t, err := typeOf(ctx, sec)
	if err != nil {
		return nil, err
	}
	switch t {
	case "stack":
		return buildStack(ctx, sec.GetArray("layers"))
	case "partition":
		return buildPartition(ctx, sec.GetArray("shards"))
	case "fs":
		dir, err := sec.Get(ctx, "dir")
		if err != nil {
			return nil, err
		}
		return fs.New(dir)
	case "sqlite":
		dsn, err := sec.Get(ctx, "dsn")
		if err != nil {
			return nil, err
		}
		return sqlite.New(dsn)
	case "mem":
		return mem.New(), nil
	case "minimal":
		return minimal.New(), nil
	case "s3":
		return buildS3(ctx, sec)
	case "azure":
		return buildAzure(ctx, sec)
	case "redis":
		return buildRedis(ctx, sec)
	case "neo4j":
		return buildNeo4j(ctx, sec)
	case "":
		return nil, fmt.Errorf("storage: missing type")
	default:
		return nil, fmt.Errorf("storage: unknown type %q", t)
	}
}

// buildStack composes ordered layer descriptors into a stack, read top-down in config
// order. Since ranke-go v0.3.0 the write role and content cap are what the universe
// reports through ranke.Capabilities, not options of the composition.
func buildStack(ctx context.Context, layers []scope.Section) (ranke.Universe, error) {
	if len(layers) == 0 {
		return nil, fmt.Errorf("storage: stack has no layers")
	}
	built := make([]ranke.Universe, 0, len(layers))
	for i, l := range layers {
		u, err := build(ctx, l)
		if err != nil {
			return nil, fmt.Errorf("storage: stack layer %d: %w", i, err)
		}
		if err := checkLayer(ctx, l, u); err != nil {
			return nil, fmt.Errorf("storage: stack layer %d: %w", i, err)
		}
		built = append(built, u)
	}
	return stack.NewStack(built...)
}

// checkLayer holds a layer's declared role and cap against what the built universe
// reports, so a wish the backend cannot honour fails rather than downgrading silently.
func checkLayer(ctx context.Context, l scope.Section, u ranke.Universe) error {
	caps := u.Capabilities()
	if l.HasValue("mode") {
		mode, err := l.Get(ctx, "mode")
		if err != nil {
			return fmt.Errorf("mode: %w", err)
		}
		want := ranke.StorageTier(mode)
		switch want {
		case ranke.StorageTierAuthoritative, ranke.StorageTierEager,
			ranke.StorageTierBackground, ranke.StorageTierLazy:
		default:
			return fmt.Errorf("mode: unknown %q (want authoritative|eager|background|lazy)", mode)
		}
		if want != caps.Tier {
			return fmt.Errorf("mode %q: this backend serves the %q tier; the tier is an adapter option in ranke-go, not a stack option — drop the key, or use a backend that offers one", want, caps.Tier)
		}
	}
	if l.HasValue("maxContentSize") {
		raw, err := l.Get(ctx, "maxContentSize")
		if err != nil {
			return fmt.Errorf("maxContentSize: %w", err)
		}
		size, err := parseSize(raw)
		if err != nil {
			return fmt.Errorf("maxContentSize: %w", err)
		}
		if size != caps.ContentCap {
			return fmt.Errorf("maxContentSize %d: this backend caps content at %d (0 = uncapped); the cap is an adapter option in ranke-go, not a stack option", size, caps.ContentCap)
		}
	}
	if l.HasValue("noReadFill") {
		v, err := l.Get(ctx, "noReadFill")
		if err != nil {
			return fmt.Errorf("noReadFill: %w", err)
		}
		if v == "true" {
			return fmt.Errorf("noReadFill: ranke-go's stack repairs a read miss itself; the switch no longer exists — remove the key")
		}
	}
	return nil
}

// buildRedis builds a redis-backed Universe: addr is required, password and db
// select the target instance's auth and database index. The client is
// constructed here since ranke-go's adapter takes one already configured.
func buildRedis(ctx context.Context, sec scope.Section) (ranke.Universe, error) {
	addr, err := sec.Get(ctx, "addr")
	if err != nil {
		return nil, fmt.Errorf("storage: redis: %w", err)
	}
	opts := &goredis.Options{Addr: addr}
	if sec.HasValue("password") {
		if opts.Password, err = sec.Get(ctx, "password"); err != nil {
			return nil, fmt.Errorf("storage: redis: %w", err)
		}
	}
	if sec.HasValue("db") {
		raw, err := sec.Get(ctx, "db")
		if err != nil {
			return nil, fmt.Errorf("storage: redis: %w", err)
		}
		if opts.DB, err = strconv.Atoi(raw); err != nil {
			return nil, fmt.Errorf("storage: redis: db: %w", err)
		}
	}
	return storageredis.New(goredis.NewClient(opts))
}

// buildS3 builds an S3-backed Universe: bucket, region, accessKeyId and
// secretAccessKey are required — credentials come from this config (as a
// literal, env(), or vault() reference), never an ambient credential chain, so
// a deployment's secrets live in one place. endpoint and usePathStyle target an
// S3-compatible service (e.g. MinIO) in place of AWS.
func buildS3(ctx context.Context, sec scope.Section) (ranke.Universe, error) {
	bucket, err := sec.Get(ctx, "bucket")
	if err != nil {
		return nil, fmt.Errorf("storage: s3: %w", err)
	}
	region, err := sec.Get(ctx, "region")
	if err != nil {
		return nil, fmt.Errorf("storage: s3: %w", err)
	}
	accessKeyID, err := sec.Get(ctx, "accessKeyId")
	if err != nil {
		return nil, fmt.Errorf("storage: s3: %w", err)
	}
	secretAccessKey, err := sec.Get(ctx, "secretAccessKey")
	if err != nil {
		return nil, fmt.Errorf("storage: s3: %w", err)
	}
	var endpoint string
	if sec.HasValue("endpoint") {
		if endpoint, err = sec.Get(ctx, "endpoint"); err != nil {
			return nil, fmt.Errorf("storage: s3: %w", err)
		}
	}
	pathStyle := false
	if sec.HasValue("usePathStyle") {
		v, err := sec.Get(ctx, "usePathStyle")
		if err != nil {
			return nil, fmt.Errorf("storage: s3: %w", err)
		}
		pathStyle = v == "true"
	}
	awsCfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	client := awss3.NewFromConfig(awsCfg, func(o *awss3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = pathStyle
	})
	return storages3.New(client, bucket)
}

// buildAzure builds an Azure Blob Storage Universe over one container, the object-store
// leaf beside s3. "readOnly" spares an immutable container the probe's sentinel write.
func buildAzure(ctx context.Context, sec scope.Section) (ranke.Universe, error) {
	name, err := sec.Get(ctx, "container")
	if err != nil {
		return nil, fmt.Errorf("storage: azure: %w", err)
	}
	client, err := blobClient(ctx, sec)
	if err != nil {
		return nil, err
	}
	var opts []storageazure.Option
	if sec.HasValue("concurrency") {
		raw, err := sec.Get(ctx, "concurrency")
		if err != nil {
			return nil, fmt.Errorf("storage: azure: concurrency: %w", err)
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("storage: azure: concurrency %q: want a positive whole number", raw)
		}
		opts = append(opts, storageazure.WithConcurrency(n))
	}
	if sec.HasValue("readOnly") {
		raw, err := sec.Get(ctx, "readOnly")
		if err != nil {
			return nil, fmt.Errorf("storage: azure: readOnly: %w", err)
		}
		if raw == "true" {
			opts = append(opts, storageazure.ReadOnly())
		}
	}
	return storageazure.New(client, name, opts...)
}

// blobClient reads the three credential forms in order: "connectionString", a shared key
// over "url", then the ambient identity over "url".
func blobClient(ctx context.Context, sec scope.Section) (*azblob.Client, error) {
	if sec.HasValue("connectionString") {
		conn, err := sec.Get(ctx, "connectionString")
		if err != nil {
			return nil, fmt.Errorf("storage: azure: connectionString: %w", err)
		}
		client, err := azblob.NewClientFromConnectionString(conn, nil)
		if err != nil {
			return nil, fmt.Errorf("storage: azure: %w", err)
		}
		return client, nil
	}
	url, err := sec.Get(ctx, "url")
	if err != nil {
		return nil, fmt.Errorf("storage: azure: url: %w (the blob service URL, or give a connectionString)", err)
	}
	if sec.HasValue("accountKey") {
		account, err := sec.Get(ctx, "accountName")
		if err != nil {
			return nil, fmt.Errorf("storage: azure: accountName: %w (required beside accountKey)", err)
		}
		key, err := sec.Get(ctx, "accountKey")
		if err != nil {
			return nil, fmt.Errorf("storage: azure: accountKey: %w", err)
		}
		cred, err := azblob.NewSharedKeyCredential(account, key)
		if err != nil {
			return nil, fmt.Errorf("storage: azure: shared key: %w", err)
		}
		client, err := azblob.NewClientWithSharedKeyCredential(url, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("storage: azure: %w", err)
		}
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("storage: azure: default credential: %w", err)
	}
	client, err := azblob.NewClient(url, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("storage: azure: %w", err)
	}
	return client, nil
}

// buildNeo4j builds a graph-native cache Universe over a neo4j driver: uri is
// required; username and password are required together, else the driver
// connects with no auth; database selects a non-default database.
func buildNeo4j(ctx context.Context, sec scope.Section) (ranke.Universe, error) {
	uri, err := sec.Get(ctx, "uri")
	if err != nil {
		return nil, fmt.Errorf("storage: neo4j: %w", err)
	}
	auth := neo4jdriver.NoAuth()
	if sec.HasValue("username") || sec.HasValue("password") {
		user, err := sec.Get(ctx, "username")
		if err != nil {
			return nil, fmt.Errorf("storage: neo4j: %w", err)
		}
		pass, err := sec.Get(ctx, "password")
		if err != nil {
			return nil, fmt.Errorf("storage: neo4j: %w", err)
		}
		auth = neo4jdriver.BasicAuth(user, pass, "")
	}
	driver, err := neo4jdriver.NewDriverWithContext(uri, auth)
	if err != nil {
		return nil, fmt.Errorf("storage: neo4j: %w", err)
	}
	var opts []storageneo4j.Option
	if sec.HasValue("database") {
		db, err := sec.Get(ctx, "database")
		if err != nil {
			return nil, fmt.Errorf("storage: neo4j: %w", err)
		}
		opts = append(opts, storageneo4j.WithDatabase(db))
	}
	return storageneo4j.New(driver, opts...), nil
}

// buildPartition routes content by id mod shard-count. Shards are bare universes.
func buildPartition(ctx context.Context, shards []scope.Section) (ranke.Universe, error) {
	if len(shards) == 0 {
		return nil, fmt.Errorf("storage: partition has no shards")
	}
	us := make([]ranke.Universe, 0, len(shards))
	for i, sh := range shards {
		u, err := build(ctx, sh)
		if err != nil {
			return nil, fmt.Errorf("storage: shard %d: %w", i, err)
		}
		us = append(us, u)
	}
	return partition.NewPartition(us...)
}

// typeOf reads the descriptor's "type", defaulting to "" when absent.
func typeOf(ctx context.Context, sec scope.Section) (string, error) {
	if !sec.HasValue("type") {
		return "", nil
	}
	return sec.Get(ctx, "type")
}

// parseSize parses a human-readable byte size: a bare number is bytes, or a
// number with a kb/mb/gb/tb suffix (decimal, case-insensitive, optional "b").
// An empty string is 0 (uncapped).
func parseSize(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	mult := uint64(1)
	matched := false
	for suffix, m := range map[string]uint64{"tb": 1 << 40, "gb": 1 << 30, "mb": 1 << 20, "kb": 1 << 10} {
		if strings.HasSuffix(s, suffix) {
			mult = m
			s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
			matched = true
			break
		}
	}
	if !matched {
		s = strings.TrimSpace(strings.TrimSuffix(s, "b")) // bare bytes, e.g. "4096b"
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n * mult, nil
}
