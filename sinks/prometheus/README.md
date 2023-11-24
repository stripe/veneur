# Prometheus remote write (RW) sink

This synk allows flushing data to a compatible prometheus remote write endpoint. 

# Configuration
The sync supports the following configuration parameters, and they can be configured in 2 ways:
- Providing value in `config.yaml` file
- Setting environment variables

## Configuration with yaml file

### Required parameters
These parameters must be provided to correctly enble the RW sink.
#### `prometheus_remote_write_address`

  The URL to be used as the remote write destination. 
  
  Example configuration: 
  ```
  prometheus_remote_write_address: "https://destination.api/prometheus/remote/write"
  ```

#### `prometheus_remote_bearer_token`

  The bearer token to use to authenticate to the provided endpoint. 
  
  Example configuration:
  ```
  prometheus_remote_bearer_token: "bearer_token"
  ```

### Optional parameters
The following parameters can be configured but the application will work with the defined defaults if they are not specified.


#### `prometheus_remote_flush_max_per_body`

  This says how many metrics to include in the body of each Remote Write request. A sample will be split into multiple ones that will be sent in parallel if the limit is exceeded. 
  
  Default value:
  ```
  prometheus_remote_flush_max_per_body: 5000
  ```

#### `prometheus_remote_flush_max_concurrency`

  Maximum concurrency of parallel writes. Veneur will queue requests above this limit when sending parallel requests.
  
  Default value:
  ```
  prometheus_remote_flush_max_concurrency: 10
  ```

#### `prometheus_remote_buffer_queue_size`

  The size of the buffer queue (in MB) for failing requests (to be retried) in case of some temporary issues with the RW endpoint.
  
  
  Default value:
  ```
  prometheus_remote_buffer_queue_size: 512
  ```

#### `prometheus_remote_flush_interval`

  Backoff time (in milliseconds) with which we will send requests to the RW endpoint.
  
  
  Default value:
  ```
  prometheus_remote_flush_interval: 1000
  ```


#### `prometheus_remote_flush_timeout`

  Client timeout (in seconds) for each individual request sent to the RW endpoint.
  
  Default value:
  ```
  prometheus_remote_flush_timeout: 35
  ```

## Configuration via environment variables

The same values can be configured with specifying environment variables, for instance via k8s deployment configuration. Here are some example values:
```
    - name: VENEUR_PROMETHEUSREMOTEWRITEADDRESS
      value: "https://destination.api/prometheus/remote/write"

    - name: VENEUR_PROMETHEUSREMOTEBEARERTOKEN
      value: "bearer_token"
    
    - name: VENEUR_PROMETHEUSREMOTEFLUSHMAXPERBODY
      value: 5000

    - name: VENEUR_PROMETHEUSREMOTEFLUSHMAXCONCURRENCY
      value: 10

    - name: VENEUR_PROMETHEUSREMOTEBUFFERQUEUESIZE
      value: 512

    - name: VENEURPROMETHEUSREMOTEFLUSHINTERVAL
      value: 1000

    - name: VENEUR_PROMETHEUSREMOTEFLUSHTIMEOUT
      value: 35
```

# Format

Metrics are published in the [specified](https://prometheus.io/docs/concepts/remote_write_spec/#protocol) standard encoded in google protobuf 3 version.