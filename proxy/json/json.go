package json

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/tdigest"
)

type JsonParseError struct {
	Err     error
	Message string
	Status  int
	Tag     string
}

func (err JsonParseError) String() string {
	return fmt.Sprintf("%s: %v", err.Message, err.Err)
}

func ParseRequest(
	request *http.Request,
) ([]samplers.JSONMetric, *JsonParseError) {
	var body io.ReadCloser
	var err error
	encoding := request.Header.Get("Content-Encoding")
	switch encoding {
	case "":
		body = request.Body
	case "deflate":
		body, err = zlib.NewReader(request.Body)
		if err != nil {
			return nil, &JsonParseError{
				Err:     err,
				Message: "failed to zlib decode body",
				Status:  http.StatusBadRequest,
				Tag:     "error_zlib",
			}
		}
		defer body.Close()
	default:
		return nil, &JsonParseError{
			Err:     fmt.Errorf("unknown Content-Encoding: %s", encoding),
			Message: "invalid request",
			Status:  http.StatusUnsupportedMediaType,
			Tag:     "error_unknown_encoding",
		}
	}

	jsonMetrics := []samplers.JSONMetric{}
	err = json.NewDecoder(body).Decode(&jsonMetrics)
	if err != nil {
		return nil, &JsonParseError{
			Err:     err,
			Message: "failed to json decode body",
			Status:  http.StatusBadRequest,
			Tag:     "error_decode",
		}
	}

	if len(jsonMetrics) == 0 {
		return nil, &JsonParseError{
			Err:     errors.New("received empty import request"),
			Message: "invalid request",
			Status:  http.StatusBadRequest,
			Tag:     "error_empty",
		}
	}

	return jsonMetrics, nil
}

func ConvertJsonMetric(metric *samplers.JSONMetric) (*metricpb.Metric, error) {
	switch metric.Type {
	case "counter":
		var value int64
		err := binary.Read(
			bytes.NewReader(metric.Value), binary.LittleEndian, &value)
		if err != nil {
			return nil, err
		}
		return &metricpb.Metric{
			Name: metric.Name,
			Tags: metric.Tags,
			Type: metricpb.Type_Counter,
			Value: &metricpb.Metric_Counter{
				Counter: &metricpb.CounterValue{
					Value: value,
				},
			},
			Scope: metricpb.Scope_Global,
		}, nil
	case "gauge":
		var value float64
		err := binary.Read(
			bytes.NewReader(metric.Value), binary.LittleEndian, &value)
		if err != nil {
			return nil, err
		}
		return &metricpb.Metric{
			Name: metric.Name,
			Tags: metric.Tags,
			Type: metricpb.Type_Gauge,
			Value: &metricpb.Metric_Gauge{
				Gauge: &metricpb.GaugeValue{
					Value: value,
				},
			},
			Scope: metricpb.Scope_Global,
		}, nil
	case "set":
		return &metricpb.Metric{
			Name: metric.Name,
			Tags: metric.Tags,
			Type: metricpb.Type_Set,
			Value: &metricpb.Metric_Set{
				Set: &metricpb.SetValue{
					HyperLogLog: metric.Value,
				},
			},
			Scope: metricpb.Scope_Global,
		}, nil
	case "histogram", "timer":
		value := tdigest.NewMerging(100, false)
		err := value.GobDecode(metric.Value)
		if err != nil {
			return nil, err
		}
		return &metricpb.Metric{
			Name: metric.Name,
			Tags: metric.Tags,
			Type: metricpb.Type_Histogram,
			Value: &metricpb.Metric_Histogram{
				Histogram: &metricpb.HistogramValue{
					TDigest: value.Data(),
				},
			},
			Scope: metricpb.Scope_Global,
		}, nil
	default:
		return nil, fmt.Errorf("unknown metric type: %s", metric.Type)
	}
}
