package logkey

import "github.com/signalfx/golib/log"

// ignored exists so that I can get some kind of coverage for this package
func ignored() string {
	// ignored
	return ""
}

var (
	// ArrLen is the len of an array
	ArrLen = log.Key("arr_len")
	// Func is the function a method was called inside
	Func = log.Key("func")

	// ZkEvent is the event object from ZK
	ZkEvent = log.Key("event")
	// ZkMethod is the zk method we are logging
	ZkMethod = log.Key("zk_method")
	// ZkPath is the path inside zk
	ZkPath = log.Key("zk_path")
	// ZkPrefix is the prefix appended to path operations
	ZkPrefix = log.Key("zk_prefix")

	// DistconfBacking is the type of distconf backing
	DistconfBacking = log.Key("distconf_backing")
	// DistconfKey is the string key of the distconf value
	DistconfKey = log.Key("distconf_key")
	// DistconfNewVal is the new distconf value we are trying to update a key to
	DistconfNewVal = log.Key("distconf_newval")

	// DiscoService is the name of a service in disco
	DiscoService = log.Key("service")
	// DiscoNode is the name of an ephemeral node in disco
	DiscoNode = log.Key("node")
	// GUID is the ID attached to a disco advertiser
	GUID = log.Key("GUID")
	// Protocol is the method of sending information
	Protocol = log.Key("protocol")

	// ExplorableParts are the parts array being used by explorable
	ExplorableParts = log.Key("parts")
	// URL is a URL endpoint
	URL = log.Key("url")

	// Endpoint is the URL endpoint that metrics go to
	Endpoint = log.Key("endpoint")
	// Name is the name of some item or group of things
	Name = log.Key("name")

	// PublishAddr is the address a server listens on
	PublishAddr = log.Key("publishAddr")
	// Size is the length of some set
	Size = log.Key("size")
	// Index is an integer index into an array
	Index = log.Key("index")
	// RetryAttempt is an index retrying an action
	RetryAttempt = log.Key("retry_attempt")
	// Env is the os environment
	Env = log.Key("env")
	// ConnCount is a count of connections
	ConnCount = log.Key("connection_count")
	// Time that a log event happens at
	Time = log.Key("time")
	// Caller is the file/line of the log statement
	Caller = log.Key("caller")
)
