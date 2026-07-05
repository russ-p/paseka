package bus

import (
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

func streamName(slug string) string {
	return "PASEKA_" + sanitizeName(slug) + "_EVENTS"
}

func kvBucketName(slug string) string {
	return "paseka_" + sanitizeName(slug) + "_task_ledger"
}

// TaskLedgerBucket returns the JetStream KV bucket name for a colony slug.
func TaskLedgerBucket(slug string) string {
	return kvBucketName(slug)
}

// ArtifactsBucket returns the JetStream object store bucket name for a colony slug.
func ArtifactsBucket(slug string) string {
	return objectStoreName(slug)
}

func objectStoreName(slug string) string {
	return "paseka_" + sanitizeName(slug) + "_artifacts"
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "colony"
	}
	return out
}

func ensureStream(js nats.JetStreamContext, cfg Config) error {
	name := streamName(cfg.Slug)
	subject := EventsWildcard(cfg.SubjectPrefix)
	if _, err := js.StreamInfo(name); err == nil {
		return nil
	}
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      name,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("bus: ensure stream %q: %w", name, err)
	}
	return nil
}

func ensureKVBuckets(js nats.JetStreamContext, cfg Config) error {
	bucket := kvBucketName(cfg.Slug)
	if _, err := js.KeyValue(bucket); err == nil {
		return nil
	}
	_, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: bucket,
	})
	if err != nil {
		return fmt.Errorf("bus: ensure kv bucket %q: %w", bucket, err)
	}
	return nil
}

func ensureObjectStore(js nats.JetStreamContext, cfg Config) error {
	name := objectStoreName(cfg.Slug)
	if _, err := js.ObjectStore(name); err == nil {
		return nil
	}
	_, err := js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket: name,
	})
	if err != nil {
		return fmt.Errorf("bus: ensure object store %q: %w", name, err)
	}
	return nil
}
