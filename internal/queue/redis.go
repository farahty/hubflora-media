package queue

import (
	"fmt"
	"net/url"

	"github.com/hibiken/asynq"
)

// ParseRedisURL parses a redis:// URL into asynq.RedisClientOpt.
// Supports: redis://host:port, redis://:password@host:port, redis://:password@host:port/db
func ParseRedisURL(redisURL string) (asynq.RedisClientOpt, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("invalid Redis URL: %w", err)
	}

	if u.Scheme != "redis" {
		return asynq.RedisClientOpt{}, fmt.Errorf("invalid Redis URL scheme: %q (expected redis://)", u.Scheme)
	}

	host := u.Host
	if host == "" {
		host = "localhost:6379"
	}

	opt := asynq.RedisClientOpt{Addr: host}

	// Password
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			opt.Password = pw
		}
	}

	// Database number from path (e.g. /1)
	if u.Path != "" && u.Path != "/" {
		var db int
		if _, err := fmt.Sscanf(u.Path, "/%d", &db); err == nil {
			opt.DB = db
		}
	}

	return opt, nil
}
