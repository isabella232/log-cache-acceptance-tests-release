package main

import (
	"context"
	"log"
	"sync"
	"time"

	"net/http"

	envstruct "code.cloudfoundry.org/go-envstruct"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/log-cache/pkg/rpc/logcache_v1"
	"google.golang.org/grpc"
)

func main() {
	log.Print("Starting LogCache Emitter...")
	defer log.Print("Closing LogCache Emitter.")

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("invalid configuration: %s", err)
	}

	envstruct.WriteReport(cfg)

	http.HandleFunc("/emit-logs", handler(cfg, emitLogs))
	http.HandleFunc("/emit-gauges", handler(cfg, emitGauges))

	log.Printf("Listening on: %s", cfg.Addr)
	http.ListenAndServe(cfg.Addr, nil)
}

func handler(cfg *Config, emitter func(*Config, []string)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		sourceIDs, ok := q["sourceIDs"]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("sourceIDs are required\n"))
			return
		}

		emitter(cfg, sourceIDs)
	}
}

func emitLogs(cfg *Config, sourceIDs []string) {
	wg := &sync.WaitGroup{}
	for _, s := range sourceIDs {
		wg.Add(1)
		go sendLogs(cfg, wg, s)
	}
	wg.Wait()
}

func sendLogs(cfg *Config, wg *sync.WaitGroup, sourceID string) {
	conn, err := grpc.Dial(cfg.LogCacheAddr, grpc.WithTransportCredentials(
		cfg.LogCacheTLS.Credentials("log-cache"),
	))
	if err != nil {
		log.Fatalf("failed to dial %s: %s", cfg.LogCacheAddr, err)
	}

	client := logcache_v1.NewIngressClient(conn)
	log.Printf("Emitting Logs for %s", sourceID)
	for i := 0; i < 10000; i++ {
		batch := []*loggregator_v2.Envelope{
			{
				Timestamp: time.Now().UnixNano(),
				SourceId:  sourceID,
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{
						Payload: []byte("log message"),
						Type:    loggregator_v2.Log_OUT,
					},
				},
			},
		}
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := client.Send(ctx, &logcache_v1.SendRequest{
			Envelopes: &loggregator_v2.EnvelopeBatch{
				Batch: batch,
			},
		})

		if err != nil {
			log.Printf("failed to write envelopes: %s", err)
			continue
		}
		time.Sleep(time.Millisecond)
	}

	wg.Done()
	log.Print("Done")
}

func emitGauges(cfg *Config, sourceIDs []string) {
	wg := &sync.WaitGroup{}
	for _, s := range sourceIDs {
		wg.Add(1)
		go sendGauges(cfg, wg, s)
	}
	wg.Wait()
}

func sendGauges(cfg *Config, wg *sync.WaitGroup, sourceID string) {
	conn, err := grpc.Dial(cfg.LogCacheAddr, grpc.WithTransportCredentials(
		cfg.LogCacheTLS.Credentials("log-cache"),
	))
	if err != nil {
		log.Fatalf("failed to dial %s: %s", cfg.LogCacheAddr, err)
	}

	client := logcache_v1.NewIngressClient(conn)
	log.Printf("Emitting Gauges for %s", sourceID)
	for i := 0; i < 10000; i++ {
		batch := []*loggregator_v2.Envelope{
			{
				Timestamp: time.Now().UnixNano(),
				SourceId:  sourceID,
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"metric": &loggregator_v2.GaugeValue{
								Value: 10.0,
								Unit:  "ms",
							},
						},
					},
				},
			},
		}

		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := client.Send(ctx, &logcache_v1.SendRequest{
			Envelopes: &loggregator_v2.EnvelopeBatch{
				Batch: batch,
			},
		})

		if err != nil {
			log.Printf("failed to write envelopes: %s", err)
			continue
		}
		time.Sleep(time.Millisecond)
	}

	wg.Done()
	log.Print("Done")
}
