package maestro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadInvalidConfig(t *testing.T) {
	_, err := (&Loader{}).Load("/doesnotexit.txt")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "python") || strings.Contains(err.Error(), "maestro.__main__"))
}

func TestReadInvalidJson(t *testing.T) {
	l := &Loader{
		ExecFunc: func(name string, stdin string, args ...string) (string, string, error) {
			return "Invalid_json", "", nil
		},
	}
	_, err := l.Load("/doesnotexit.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

func TestBadArbitrary(t *testing.T) {
	var m map[string]Instance
	assert.Panics(t, func() {
		_ = ArbitraryInstance(m)
	})
}

func TestEmptyLoad(t *testing.T) {
	l := &Loader{
		ExecFunc: func(name string, stdin string, args ...string) (string, string, error) {
			return "{}", "", nil
		},
	}

	c, err := l.Load("")
	assert.NotNil(t, c)
	assert.NoError(t, err)
}

func TestShipsForService(t *testing.T) {
	c := Config{
		Name: "lab",
		Ships: map[string]Ships{
			"ship1": {
				IP: "a.b.c.d",
			},
		},
		Services: map[string]Service{
			"zookeeper": {
				Instances: map[string]Instance{
					"z1": {
						Ship: "ship1",
						Ports: map[string]Port{
							"client": float64(123),
						},
					},
				},
			},
		},
	}
	assert.Equal(t, []string{}, c.ShipsForService("test"))
	assert.Equal(t, []string{"a.b.c.d:123"}, c.ShipsForService("zookeeper"))
	assert.Equal(t, "ship1", ArbitraryInstance(c.Services["zookeeper"].Instances).Ship)
}
