package distconf

import (
	"os"
	"testing"

	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
)

func TestEnvConf(t *testing.T) {

	e, err := EnvLoader().Get()
	assert.NoError(t, err)
	b, err := e.Get("not_in_env_i_hope_SDFSDFSDFSDFSDF")
	assert.NoError(t, err)
	assert.Nil(t, b)

	log.IfErr(log.Panic, os.Setenv("test_TestEnvConf", "abc"))
	b, err = e.Get("test_TestEnvConf")
	assert.NoError(t, err)
	assert.Equal(t, []byte("abc"), b)
}
