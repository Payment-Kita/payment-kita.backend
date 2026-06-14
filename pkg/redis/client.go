package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

var pingClient = func(ctx context.Context, c *redis.Client) error {
	return c.Ping(ctx).Err()
}

// Init initializes the Redis client
func Init(url, username, password string) error {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return err
	}

	if username != "" {
		opts.Username = username
	}

	if password != "" {
		opts.Password = password
	}

	client = redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pingClient(ctx, client); err != nil {
		return err
	}

	return nil
}

// SetClient sets the Redis client (used for testing)
func SetClient(c *redis.Client) {
	client = c
}

// GetClient returns the Redis client
func GetClient() *redis.Client {
	return client
}

// Set stores a key-value pair with expiration
func Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key
func Get(ctx context.Context, key string) (string, error) {
	return client.Get(ctx, key).Result()
}

// Del removes a key
func Del(ctx context.Context, key string) error {
	return client.Del(ctx, key).Err()
}

// SetNX sets a key only if it does not exist
func SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	return client.SetNX(ctx, key, value, expiration).Result()
}

// SetEX sets a key with value and expiration (alias for Set)
func SetEX(ctx context.Context, key string, value string, expiration time.Duration) error {
	return client.Set(ctx, key, value, expiration).Err()
}

// Incr increments a key
func Incr(ctx context.Context, key string) (int64, error) {
	return client.Incr(ctx, key).Result()
}

// Expire sets expiration for a key
func Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return client.Expire(ctx, key, expiration).Result()
}
