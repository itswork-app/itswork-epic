package cache

import (
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
)

func TestInitRedis_Success(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	os.Setenv("REDIS_URL", "redis://"+mr.Addr())
	defer os.Unsetenv("REDIS_URL")

	client, err := InitRedis()
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestInitRedis_MissingEnv(t *testing.T) {
	os.Unsetenv("REDIS_URL")
	client, err := InitRedis()
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestInitRedis_InvalidURL(t *testing.T) {
	os.Setenv("REDIS_URL", "not-a-url \\x00")
	defer os.Unsetenv("REDIS_URL")

	client, err := InitRedis()
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestInitRedis_PingFailed(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)

	os.Setenv("REDIS_URL", "redis://"+mr.Addr())
	defer os.Unsetenv("REDIS_URL")

	mr.Close() // Close immediately to trigger Dial/Ping failure

	client, err := InitRedis()
	assert.Error(t, err)
	assert.Nil(t, client)
}
