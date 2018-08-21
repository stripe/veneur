package hec

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/satori/go.uuid"
)

const (
	retryWaitTime = 1 * time.Second

	defaultMaxContentLength = 1000000
)

type Client struct {
	HEC

	// HTTP Client for communication with (optional)
	httpClient *http.Client

	// Splunk Server URL for API requests (required)
	serverURL string

	// HEC Token (required)
	token string

	// Keep-Alive (optional, default: true)
	keepAlive bool

	// Channel (required for Raw mode)
	channel string

	// Max retrying times (optional, default: 2)
	retries int

	// Max content length (optional, default: 1000000)
	maxLength int
}

func NewClient(serverURL string, token string) HEC {
	id, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	return &Client{
		httpClient: http.DefaultClient,
		serverURL:  serverURL,
		token:      token,
		keepAlive:  true,
		channel:    id.String(),
		retries:    2,
		maxLength:  defaultMaxContentLength,
	}
}

func (hec *Client) SetHTTPClient(client *http.Client) {
	hec.httpClient = client
}

func (hec *Client) SetKeepAlive(enable bool) {
	hec.keepAlive = enable
}

func (hec *Client) SetChannel(channel string) {
	hec.channel = channel
}

func (hec *Client) SetMaxRetry(retries int) {
	hec.retries = retries
}

func (hec *Client) SetMaxContentLength(size int) {
	hec.maxLength = size
}

func (hec *Client) WriteEventWithContext(ctx context.Context, event *Event) error {
	if event.empty() {
		return nil // skip empty events
	}

	endpoint := "/services/collector?channel=" + hec.channel
	data, _ := json.Marshal(event)

	if len(data) > hec.maxLength {
		return ErrEventTooLong
	}
	return hec.write(ctx, endpoint, data)
}

func (hec *Client) WriteEvent(event *Event) error {
	return hec.WriteEventWithContext(context.Background(), event)
}

func (hec *Client) WriteBatchWithContext(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	endpoint := "/services/collector?channel=" + hec.channel
	var buffer bytes.Buffer
	var tooLongs []int

	for index, event := range events {
		if event.empty() {
			continue // skip empty events
		}

		data, _ := json.Marshal(event)
		if len(data) > hec.maxLength {
			tooLongs = append(tooLongs, index)
			continue
		}
		// Send out bytes in buffer immediately if the limit exceeded after adding this event
		if buffer.Len()+len(data) > hec.maxLength {
			if err := hec.write(ctx, endpoint, buffer.Bytes()); err != nil {
				return err
			}
			buffer.Reset()
		}
		buffer.Write(data)
	}

	if buffer.Len() > 0 {
		if err := hec.write(ctx, endpoint, buffer.Bytes()); err != nil {
			return err
		}
	}
	if len(tooLongs) > 0 {
		return ErrEventTooLong
	}
	return nil
}

func (hec *Client) WriteBatch(events []*Event) error {
	return hec.WriteBatchWithContext(context.Background(), events)
}

type EventMetadata struct {
	Host       *string
	Index      *string
	Source     *string
	SourceType *string
	Time       *time.Time
}

func (hec *Client) WriteRawWithContext(ctx context.Context, reader io.ReadSeeker, metadata *EventMetadata) error {
	endpoint := rawHecEndpoint(hec.channel, metadata)

	return breakStream(reader, hec.maxLength, func(chunk []byte) error {
		if err := hec.write(ctx, endpoint, chunk); err != nil {
			// Ignore NoData error (e.g. "\n\n" will cause NoData error)
			if res, ok := err.(*Response); !ok || res.Code != StatusNoData {
				return err
			}
		}
		return nil
	})
}

func (hec *Client) WriteRaw(reader io.ReadSeeker, metadata *EventMetadata) error {
	return hec.WriteRawWithContext(context.Background(), reader, metadata)
}

// breakStream breaks text from reader into chunks, with every chunk less than max.
// Unless a single line is longer than max, it always cut at end of lines ("\n")
func breakStream(reader io.ReadSeeker, max int, callback func(chunk []byte) error) error {

	var buf []byte = make([]byte, max+1)
	var writeAt int
	for {
		n, err := reader.Read(buf[writeAt:max])
		if n == 0 && err == io.EOF {
			break
		}

		// If last line does not end with LF, add one for it
		if err == io.EOF && buf[writeAt+n-1] != '\n' {
			n++
			buf[writeAt+n-1] = '\n'
		}

		data := buf[0 : writeAt+n]

		// Cut after the last LF character
		cut := bytes.LastIndexByte(data, '\n') + 1
		if cut == 0 {
			// This line is too long, but just let it break here
			cut = len(data)
		}
		if err := callback(buf[:cut]); err != nil {
			return err
		}

		writeAt = copy(buf, data[cut:])

		if err != nil && err != io.EOF {
			return err
		}
	}

	if writeAt != 0 {
		return callback(buf[:writeAt])
	}

	return nil
}

func responseFrom(body []byte) *Response {
	var res Response
	json.Unmarshal(body, &res)
	return &res
}

func (res *Response) Error() string {
	return res.Text
}

func (res *Response) String() string {
	b, _ := json.Marshal(res)
	return string(b)
}

func (hec *Client) write(ctx context.Context, endpoint string, data []byte) error {
	retries := 0
RETRY:
	req, err := http.NewRequest(http.MethodPost, hec.serverURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	if hec.keepAlive {
		req.Header.Set("Connection", "keep-alive")
	}
	req.Header.Set("Authorization", "Splunk "+hec.token)
	res, err := hec.httpClient.Do(req)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		response := responseFrom(body)
		if retriable(response.Code) && retries < hec.retries {
			retries++
			time.Sleep(retryWaitTime)
			goto RETRY
		}
		return response
	}
	return nil
}

func rawHecEndpoint(channel string, metadata *EventMetadata) string {
	var buffer bytes.Buffer
	buffer.WriteString("/services/collector/raw?channel=" + channel)
	if metadata == nil {
		return buffer.String()
	}
	if metadata.Host != nil {
		buffer.WriteString("&host=" + *metadata.Host)
	}
	if metadata.Index != nil {
		buffer.WriteString("&index=" + *metadata.Index)
	}
	if metadata.Source != nil {
		buffer.WriteString("&source=" + *metadata.Source)
	}
	if metadata.SourceType != nil {
		buffer.WriteString("&sourcetype=" + *metadata.SourceType)
	}
	if metadata.Time != nil {
		buffer.WriteString("&time=" + epochTime(metadata.Time))
	}
	return buffer.String()
}
