package ringkeeper

import (
	"net"
	"strconv"

	"github.com/pkg/errors"
)

// DNSSRV uses a DNS SRV request to discover services.
func DNSSRV(service string) ([]string, error) {
	_, addrs, err := net.LookupSRV("", "", service)
	if err != nil {
		return nil, errors.WithMessage(err, "failed performing DNSSRV discovery")
	}

	var hostports []string
	for _, addr := range addrs {
		hostports = append(
			hostports,
			addr.Target[:len(addr.Target)-1]+":"+strconv.Itoa(int(addr.Port)),
		)
	}
	return hostports, nil
}
