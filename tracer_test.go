// Copyright 2025 zampo.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	"context"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServiceName != "ai-api-market" {
		t.Errorf("DefaultConfig() ServiceName = %q, want %q", cfg.ServiceName, "ai-api-market")
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("DefaultConfig() ServiceVersion = %q, want %q", cfg.ServiceVersion, "1.0.0")
	}
	if cfg.Environment != "development" {
		t.Errorf("DefaultConfig() Environment = %q, want %q", cfg.Environment, "development")
	}
	if cfg.OTLEndpoint != "http://localhost:4318/v1/traces" {
		t.Errorf("DefaultConfig() OTLEndpoint = %q, want %q", cfg.OTLEndpoint, "http://localhost:4318/v1/traces")
	}
	if !cfg.Enabled {
		t.Error("DefaultConfig() Enabled should be true")
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("DefaultConfig() SampleRate = %f, want %f", cfg.SampleRate, 1.0)
	}
}

func TestFromAppConfig(t *testing.T) {
	cfg := FromAppConfig(nil)

	if cfg.ServiceName != "ai-api-market" {
		t.Errorf("FromAppConfig() ServiceName = %q, want %q", cfg.ServiceName, "ai-api-market")
	}
	if !cfg.Enabled {
		t.Error("FromAppConfig() Enabled should be true")
	}
}

func TestInit_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	err := Init(cfg)
	if err != nil {
		t.Errorf("Init(disabled) unexpected error: %v", err)
	}
}

func TestInit_EmptyEndpoint(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		OTLEndpoint: "",
	}

	err := Init(cfg)
	if err == nil {
		t.Error("Init(empty endpoint) expected error, got nil")
	}
	if err.Error() != "otlp endpoint is required when tracing is enabled" {
		t.Errorf("Init(empty endpoint) error message = %q, want %q", err.Error(), "otlp endpoint is required when tracing is enabled")
	}
}

func TestExtractHostPort_WithHTTP(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "http endpoint with path",
			endpoint: "http://localhost:4318/v1/traces",
			want:     "localhost:4318",
		},
		{
			name:     "https endpoint with path",
			endpoint: "https://localhost:4317/v1/traces",
			want:     "localhost:4317",
		},
		{
			name:     "endpoint without path",
			endpoint: "localhost:4318",
			want:     "localhost:4318",
		},
		{
			name:     "endpoint without port",
			endpoint: "localhost",
			want:     "localhost:4318",
		},
		{
			name:     "ip endpoint without port",
			endpoint: "127.0.0.1",
			want:     "127.0.0.1:4318",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHostPort(tt.endpoint)
			if got != tt.want {
				t.Errorf("extractHostPort(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestExtractHostPort_IPv6(t *testing.T) {
	result := extractHostPort("[::1]:4318")
	if result != "[::1]:4318" {
		t.Errorf("extractHostPort() = %q, want %q", result, "[::1]:4318")
	}
}

func TestShutdown_NoProvider(t *testing.T) {
	tracerProvider = nil

	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown() unexpected error: %v", err)
	}
}

func TestGetTracer_NoInit(t *testing.T) {
	tracer = nil

	tr := GetTracer()
	if tr == nil {
		t.Error("GetTracer() returned nil")
	}
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := StartSpan(ctx, "test-span")

	if span == nil {
		t.Error("StartSpan() returned nil span")
	}
	span.End()

	if ctx == nil {
		t.Error("StartSpan() returned nil context")
	}
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()
	ctx, _ = StartSpan(ctx, "test-span")

	span := SpanFromContext(ctx)
	if span == nil {
		t.Error("SpanFromContext() returned nil span")
	}
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	traceID := TraceIDFromContext(ctx)
	if traceID != "" {
		t.Errorf("TraceIDFromContext(empty) = %q, want empty string", traceID)
	}
}

func TestSpanIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	spanID := SpanIDFromContext(ctx)
	if spanID != "" {
		t.Errorf("SpanIDFromContext(empty) = %q, want empty string", spanID)
	}
}

func TestNoopExporter_ExportSpans(t *testing.T) {
	exp := &noopExporter{}
	err := exp.ExportSpans(context.Background(), nil)
	if err != nil {
		t.Errorf("noopExporter.ExportSpans() unexpected error: %v", err)
	}
}

func TestNoopExporter_Shutdown(t *testing.T) {
	exp := &noopExporter{}
	err := exp.Shutdown(context.Background())
	if err != nil {
		t.Errorf("noopExporter.Shutdown() unexpected error: %v", err)
	}
}
