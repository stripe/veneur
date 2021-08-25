// Copyright 2019 Splunk, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package translator

import (
	"crypto/sha256"

	"github.com/gogo/protobuf/proto"
	jaegerpb "github.com/jaegertracing/jaeger/model"
)

// SpanBatcher is simpler version of OpenTelemetry's Node Batcher.
// SpanBatcher takes spans and groups them into Jaeger Batches using
// the Span Process objects.
type SpanBatcher struct {
	// Buckets are a map of jaeger batches with a key generated from the jaeger process
	Buckets map[[32]byte]*jaegerpb.Batch
}

// Add jaeger spans to the batcher
func (b *SpanBatcher) Add(span *jaegerpb.Span) {
	if b.Buckets == nil {
		b.Buckets = make(map[[32]byte]*jaegerpb.Batch)
	}

	id, err := GenSpanBatcherBucketID(span.Process)
	if err == nil {
		batchByProcess := span.Process
		batch := b.getOrAddBatch(id, batchByProcess)
		if batch.Process != nil {
			span.Process = nil
		}
		batch.Spans = append(batch.Spans, span)
	}
}

// Batches returns an array of jaeger batches
func (b *SpanBatcher) Batches() []*jaegerpb.Batch {
	batches := make([]*jaegerpb.Batch, 0, len(b.Buckets))
	for _, b := range b.Buckets {
		batches = append(batches, b)
	}
	return batches
}

func (b *SpanBatcher) getOrAddBatch(id [32]byte, p *jaegerpb.Process) *jaegerpb.Batch {
	batch, ok := b.Buckets[id]
	if !ok {
		batch = &jaegerpb.Batch{
			Process: p,
		}
		b.Buckets[id] = batch
	}
	return batch
}

// GenSpanBatcherBucketID generates a SpanBatcher bucket id from a jaeger process
func GenSpanBatcherBucketID(process *jaegerpb.Process) (bid [32]byte, err error) {
	if process != nil {
		sortTags(process.Tags)
		var key []byte
		key, err = proto.Marshal(process)
		if err == nil {
			return sha256.Sum256(key), nil
		}
	}
	return bid, err
}
