package smartdns

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	redisCon "github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocking Redis connection
type MockRedisConn struct {
	mock.Mock
}

func (m *MockRedisConn) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	arguments := m.Called(commandName, args)
	return arguments.Get(0), arguments.Error(1)
}

func (m *MockRedisConn) Close() error {
	return nil
}

func TestRedisGet(t *testing.T) {
	mockConn := new(MockRedisConn)
	pool := &redisCon.Pool{
		Dial: func() (redisCon.Conn, error) {
			return mockConn, nil
		},
	}

	redis := &Redis{
		Pool:      pool,
		keyPrefix: "prefix:",
		keySuffix: ":suffix",
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "smartIsp", "isp1")
	ctx = context.WithValue(ctx, "smartProvince", "province1")

	zone := &Zone{Name: "example.com"}
	record := &Record{
		A: []A_Record{{Ip: net.ParseIP("1.2.3.4")}},
	}

	// Mock Redis HGET responses
	recordJson, _ := json.Marshal(record)
	mockConn.On("Do", "HGET", "prefix:example.com:suffix", "smart:isp1:province1").Return(string(recordJson), nil)

	result := redis.get(ctx, "example.com", zone)
	assert.NotNil(t, result)
	assert.Equal(t, "1.2.3.4", result.A[0].Ip.String())
}
