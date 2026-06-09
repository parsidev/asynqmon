package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hibiken/asynq"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		args []string
		want *Config
	}{
		{
			args: []string{"--redis-addr", "localhost:6380", "--redis-db", "3"},
			want: &Config{
				RedisAddr: "localhost:6380",
				RedisDB:   3,

				// Default values
				Port:                  8080,
				RedisPassword:         "",
				RedisTLS:              "",
				RedisURL:              "",
				RedisInsecureTLS:      false,
				RedisClusterNodes:     "",
				RedisPrefix:           "",
				MaxPayloadLength:      200,
				MaxResultLength:       200,
				EnableMetricsExporter: false,
				PrometheusServerAddr:  "",
				ReadOnly:              false,
				BasicAuthUsername:     "",
				BasicAuthPassword:     "",

				Args: []string{},
			},
		},
		{
			args: []string{"--basic-auth-username", "admin", "--basic-auth-password", "secret"},
			want: &Config{
				Port:                  8080,
				RedisAddr:             "127.0.0.1:6379",
				RedisDB:               0,
				RedisPassword:         "",
				RedisTLS:              "",
				RedisURL:              "",
				RedisInsecureTLS:      false,
				RedisClusterNodes:     "",
				RedisPrefix:           "",
				ReadOnly:              false,
				MaxPayloadLength:      200,
				MaxResultLength:       200,
				BasicAuthUsername:     "admin",
				BasicAuthPassword:     "secret",
				EnableMetricsExporter: false,
				PrometheusServerAddr:  "",
				Args:                  []string{},
			},
		},
		{
			args: []string{"--redis-prefix", "tenant-a"},
			want: &Config{
				Port:                  8080,
				RedisAddr:             "127.0.0.1:6379",
				RedisDB:               0,
				RedisPassword:         "",
				RedisTLS:              "",
				RedisURL:              "",
				RedisInsecureTLS:      false,
				RedisClusterNodes:     "",
				RedisPrefix:           "tenant-a",
				MaxPayloadLength:      200,
				MaxResultLength:       200,
				EnableMetricsExporter: false,
				PrometheusServerAddr:  "",
				ReadOnly:              false,
				BasicAuthUsername:     "",
				BasicAuthPassword:     "",
				Args:                  []string{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cfg, output, err := parseFlags("asynqmon", tc.args)
			if err != nil {
				t.Errorf("parseFlags returned error: %v", err)
			}
			if output != "" {
				t.Errorf("parseFlag returned output=%q, want empty", output)
			}
			if diff := cmp.Diff(tc.want, cfg); diff != "" {
				t.Errorf("parseFlag returned Config %v, want %v; (-want,+got)\n%s", cfg, tc.want, diff)
			}
		})
	}

}

func TestWithOptionalBasicAuth(t *testing.T) {
	handler := withOptionalBasicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), &Config{
		BasicAuthUsername: "admin",
		BasicAuthPassword: "secret",
	})

	tests := []struct {
		desc       string
		username   string
		password   string
		wantStatus int
	}{
		{desc: "missing credentials", wantStatus: http.StatusUnauthorized},
		{desc: "wrong credentials", username: "admin", password: "bad", wantStatus: http.StatusUnauthorized},
		{desc: "correct credentials", username: "admin", password: "secret", wantStatus: http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.username != "" || tc.password != "" {
				req.SetBasicAuth(tc.username, tc.password)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestWithOptionalBasicAuthDisabled(t *testing.T) {
	handler := withOptionalBasicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), &Config{
		BasicAuthUsername: "admin",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestMakeRedisConnOpt(t *testing.T) {
	var tests = []struct {
		desc string
		cfg  *Config
		want asynq.RedisConnOpt
	}{
		{
			desc: "With address, db number and password",
			cfg: &Config{
				RedisAddr:     "localhost:6380",
				RedisDB:       1,
				RedisPassword: "foo",
				RedisPrefix:   "tenant-a",
			},
			want: asynq.RedisClientOpt{
				Addr:     "localhost:6380",
				DB:       1,
				Password: "foo",
				Prefix:   "tenant-a",
			},
		},
		{
			desc: "With TLS server name",
			cfg: &Config{
				RedisAddr: "localhost:6379",
				RedisTLS:  "foobar",
			},
			want: asynq.RedisClientOpt{
				Addr:      "localhost:6379",
				TLSConfig: &tls.Config{ServerName: "foobar"},
			},
		},
		{
			desc: "With redis URL",
			cfg: &Config{
				RedisURL: "redis://:bar@localhost:6381/2",
			},
			want: asynq.RedisClientOpt{
				Addr:     "localhost:6381",
				DB:       2,
				Password: "bar",
			},
		},
		{
			desc: "With redis-sentinel URL",
			cfg: &Config{
				RedisURL: "redis-sentinel://:secretpassword@localhost:5000,localhost:5001,localhost:5002?master=mymaster",
			},
			want: asynq.RedisFailoverClientOpt{
				MasterName: "mymaster",
				SentinelAddrs: []string{
					"localhost:5000", "localhost:5001", "localhost:5002"},
				SentinelPassword: "secretpassword",
			},
		},
		{
			desc: "With cluster nodes",
			cfg: &Config{
				RedisClusterNodes: "localhost:5000,localhost:5001,localhost:5002,localhost:5003,localhost:5004,localhost:5005",
				RedisPrefix:       "tenant-a",
			},
			want: asynq.RedisClusterClientOpt{
				Addrs: []string{
					"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003", "localhost:5004", "localhost:5005"},
				Prefix: "tenant-a",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := makeRedisConnOpt(tc.cfg)
			if err != nil {
				t.Fatalf("makeRedisConnOpt returned error: %v", err)
			}

			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreUnexported(tls.Config{})); diff != "" {
				t.Errorf("diff found: want=%v, got=%v; (-want,+got)\n%s",
					tc.want, got, diff)
			}
		})
	}
}
