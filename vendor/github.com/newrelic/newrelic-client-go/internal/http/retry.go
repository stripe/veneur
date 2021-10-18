package http

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/url"
	"regexp"
)

var (
	// A regular expression to match the error returned by net/http when the
	// configured number of redirects is exhausted. This error isn't typed
	// specifically so we resort to matching on the error string.
	redirectsErrorRe = regexp.MustCompile(`stopped after \d+ redirects\z`)

	// A regular expression to match the error returned by net/http when the
	// scheme specified in the URL is invalid. This error isn't typed
	// specifically so we resort to matching on the error string.
	schemeErrorRe = regexp.MustCompile(`unsupported protocol scheme`)
)

// RetryPolicy provides a callback for retryablehttp's CheckRetry, which
// will retry on connection errors and server errors.
func RetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if err != nil {
		if v, ok := err.(*url.Error); ok {
			// Don't retry if the error was due to too many redirects.
			if redirectsErrorRe.MatchString(v.Error()) {
				return false, nil
			}

			// Don't retry if the error was due to an invalid protocol scheme.
			if schemeErrorRe.MatchString(v.Error()) {
				return false, nil
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
				return false, nil
			}
		}

		// The error is likely recoverable so retry.
		return true, nil
	}

	// Check the response code. We retry on 5xx responses to allow
	// the server time to recover, as 5xx's are typically not permanent
	// errors and may relate to outages on the server side. We are notably
	// disallowing retries for 500 errors here as the underlying APIs use them to
	// provide useful error validation messages that should be passed back to the
	// end user. This will catch invalid response codes as well, like 0 and 999.
	// 429 Too Many Requests is retried as well to handle the aggressive rate limiting
	// of the Synthetics API.
	if resp.StatusCode == 0 ||
		resp.StatusCode == 429 ||
		resp.StatusCode >= 502 {
		return true, nil
	}

	return false, nil
}
