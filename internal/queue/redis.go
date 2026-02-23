package queue

import (
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
)

// ParseRedisURL parses a redis:// URL into asynq.RedisClientOpt.
func ParseRedisURL(redisURL string) (asynq.RedisClientOpt, error) {
	// asynq expects redis:// format which it can parse natively
	if !strings.HasPrefix(redisURL, "redis://") {
		return asynq.RedisClientOpt{}, fmt.Errorf("invalid Redis URL: must start with redis://")
	}
	return asynq.RedisClientOpt{Addr: strings.TrimPrefix(redisURL, "redis://")}, nil
}
