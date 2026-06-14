package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestInitInvalidURL(t *testing.T) {
	err := Init("://invalid-url", "", "")
	assert.Error(t, err)
}

func TestInitValidURLWithPingHook(t *testing.T) {
	originalPing := pingClient
	t.Cleanup(func() { pingClient = originalPing })

	pingClient = func(_ context.Context, _ *goredis.Client) error {
		return errors.New("forced ping failure")
	}
	err := Init("redis://127.0.0.1:1", "", "secret")
	assert.Error(t, err)

	pingClient = func(_ context.Context, _ *goredis.Client) error {
		return nil
	}
	err = Init("redis://127.0.0.1:1", "", "secret")
	assert.NoError(t, err)
	assert.NotNil(t, GetClient())
}

func TestSetClientAndBasicOpsWithUnreachableRedis(t *testing.T) {
	cli := goredis.NewClient(&goredis.Options{
		Addr:         "127.0.0.1:0", // invalid/unreachable
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	SetClient(cli)
	assert.NotNil(t, GetClient())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	assert.Error(t, Set(ctx, "k", "v", time.Second))
	_, err := Get(ctx, "k")
	assert.Error(t, err)
	assert.Error(t, Del(ctx, "k"))
	_, err = SetNX(ctx, "k", "v", time.Second)
	assert.Error(t, err)
}

func TestInitAndBasicOpsSuccessWithMiniRedis(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Skipf("skip: miniredis unavailable in this environment: %v", err)
	}
	defer srv.Close()

	err = Init("redis://"+srv.Addr(), "", "")
	assert.NoError(t, err)
	assert.NotNil(t, GetClient())

	ctx := context.Background()
	assert.NoError(t, Set(ctx, "k1", "v1", time.Minute))
	got, err := Get(ctx, "k1")
	assert.NoError(t, err)
	assert.Equal(t, "v1", got)

	ok, err := SetNX(ctx, "k1", "v2", time.Minute)
	assert.NoError(t, err)
	assert.False(t, ok)

	ok, err = SetNX(ctx, "k2", "v2", time.Minute)
	assert.NoError(t, err)
	assert.True(t, ok)

	assert.NoError(t, Del(ctx, "k1"))
	_, err = Get(ctx, "k1")
	assert.Error(t, err)
}
