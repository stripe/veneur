package distconf

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
)

func TestBadIniLoad(t *testing.T) {
	_, err := Ini("/asdfdsaf/asdf/dsfad/sdfsa/fdsa/fasd/dsfa/sdfa/")
	assert.Error(t, err)
}

func TestIniConf(t *testing.T) {

	file, err := ioutil.TempFile("", "TestIniConf")
	assert.NoError(t, err)
	defer func() {
		log.IfErr(log.Panic, os.Remove(file.Name()))
	}()
	log.IfErr(log.Panic, file.Close())
	assert.NoError(t, ioutil.WriteFile(file.Name(), []byte(`
val1=abc
	`), 0))

	i, err := IniLoader(file.Name()).Get()
	assert.NoError(t, err)
	b, err := i.Get("not_in_env_i_hope_SDFSDFSDFSDFSDF")
	assert.NoError(t, err)
	assert.Nil(t, b)

	b, err = i.Get("val1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("abc"), b)
}
