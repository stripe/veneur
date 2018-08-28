package hec

import (
	"io"
	"math/rand"
	"net/http"
	"sync"

	"github.com/satori/go.uuid"
)

type Cluster struct {
	HEC

	// Inner clients
	clients []*Client

	mtx sync.Mutex

	maxRetries int
}

func NewCluster(serverURLs []string, token string) HEC {
	id, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	channel := id.String()
	clients := make([]*Client, len(serverURLs))
	for i, serverURL := range serverURLs {
		clients[i] = &Client{
			httpClient: http.DefaultClient,
			serverURL:  serverURL,
			token:      token,
			keepAlive:  true,
			channel:    channel,
			retries:    0, // try only once for each client
			maxLength:  defaultMaxContentLength,
		}
	}
	return &Cluster{
		clients:    clients,
		maxRetries: -1, // default: try all clients
	}
}

func (c *Cluster) SetHTTPClient(httpClient *http.Client) {
	c.mtx.Lock()
	for _, client := range c.clients {
		client.SetHTTPClient(httpClient)
	}
	c.mtx.Unlock()
}

func (c *Cluster) SetKeepAlive(enable bool) {
	c.mtx.Lock()
	for _, client := range c.clients {
		client.SetKeepAlive(enable)
	}
	c.mtx.Unlock()
}

func (c *Cluster) SetChannel(channel string) {
	c.mtx.Lock()
	for _, client := range c.clients {
		client.SetChannel(channel)
	}
	c.mtx.Unlock()
}

func (c *Cluster) SetMaxRetry(retries int) {
	c.maxRetries = retries
}

func (c *Cluster) SetMaxContentLength(size int) {
	c.mtx.Lock()
	for _, client := range c.clients {
		client.SetMaxContentLength(size)
	}
	c.mtx.Unlock()
}

func (c *Cluster) WriteEvent(event *Event) error {
	return c.retry(func(client *Client) error {
		return client.WriteEvent(event)
	})
}

func (c *Cluster) WriteBatch(events []*Event) error {
	return c.retry(func(client *Client) error {
		return client.WriteBatch(events)
	})
}

func (c *Cluster) WriteRaw(reader io.ReadSeeker, metadata *EventMetadata) error {
	startAt, _ := reader.Seek(0, io.SeekCurrent)
	return c.retry(func(client *Client) error {
		reader.Seek(startAt, io.SeekStart)
		return client.WriteRaw(reader, metadata)
	})
}

func (c *Cluster) retry(writeFunc func(*Client) error) error {
	exclude := make([]*Client, 0)
	var err error
	for t := 0; t < len(c.clients) && t != c.maxRetries; t++ {
		client := pick(c.clients, exclude)
		if err = writeFunc(client); err != nil {
			if err == ErrEventTooLong {
				return err
			} else if res, ok := err.(*Response); !ok || retriable(res.Code) {
				// If failed to write into this client, exclude it and try others
				exclude = append(exclude, client)
				continue
			}
		} else {
			return nil
		}
	}
	return err
}

func pick(clients []*Client, exclude []*Client) *Client {
	var choice *Client
	for choice == nil {
		choice = clients[rand.Int()%len(clients)]
		if exclude == nil {
			break
		}
		for _, bad := range exclude {
			if bad == choice {
				choice = nil
				break
			}
		}
	}
	return choice
}
