package build

import "net/http"

const defaultValue = "dirty"

var (
	BUILD_DATE = defaultValue
	VERSION    = defaultValue
)

func HandleBuildDate(writer http.ResponseWriter, _ *http.Request) {
	writer.Write([]byte(BUILD_DATE))
}

func HandleVersion(writer http.ResponseWriter, _ *http.Request) {
	writer.Write([]byte(VERSION))
}
