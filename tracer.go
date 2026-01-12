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
//
// @contact  zampo3380@gmail.com

package trace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-anyway/framework-log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	// TracerProvider 全局追踪提供者
	tracerProvider *tracesdk.TracerProvider
	// Tracer 全局追踪器
	tracer trace.Tracer
)

// Config 追踪配置
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLEndpoint    string // OTLP endpoint, 例如: http://localhost:4318/v1/traces (HTTP) 或 http://localhost:4317 (gRPC)
	Enabled        bool
	SampleRate     float64 // 采样率 0.0-1.0
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		ServiceName:    "ai-api-market",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		OTLEndpoint:    "http://localhost:4318/v1/traces", // OTLP HTTP endpoint (Jaeger 从 v1.35.0 开始支持)
		Enabled:        true,
		SampleRate:     1.0,
	}
}

// FromAppConfig 从 app.TracingConfig 创建 trace.Config
func FromAppConfig(appCfg interface{}) Config {
	// 使用反射或类型断言获取配置
	// 这里简化处理，实际使用时需要根据具体类型转换
	return DefaultConfig()
}

// Init 初始化 OpenTelemetry 追踪
func Init(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}

	// 验证 OTLP 端点配置
	if cfg.OTLEndpoint == "" {
		return fmt.Errorf("otlp endpoint is required when tracing is enabled")
	}

	// 创建资源
	// 注意：不使用 resource.Default() 避免 Schema URL 冲突
	// 直接创建 resource 并指定统一的 Schema URL
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironment(cfg.Environment),
	)

	// 创建 OTLP HTTP exporter
	// 注意：如果端点不存在或连接失败，使用 noopExporter 避免内存泄漏
	// OTLP HTTP endpoint 格式: host:port (例如: localhost:4318)
	// 路径 /v1/traces 会自动添加
	var exp tracesdk.SpanExporter
	otlpExp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(extractHostPort(cfg.OTLEndpoint)),
		otlptracehttp.WithInsecure(), // 如果使用 HTTPS，需要配置 TLS
	)
	if err != nil {
		// 如果创建 exporter 失败，不阻止启动，但记录错误
		// 使用 NoopExporter 避免内存泄漏
		log.Warn("Failed to create OTLP exporter, using noop exporter", zap.Error(err))
		exp = &noopExporter{}
	} else {
		exp = otlpExp
	}

	// 限制采样率范围
	sampleRate := cfg.SampleRate
	if sampleRate < 0 {
		sampleRate = 0
	}
	if sampleRate > 1 {
		sampleRate = 1
	}

	// 创建采样器
	// 使用 ParentBased sampler 来继承父 span 的采样决策
	// 对于根 span（没有父 span），使用 TraceIDRatioBased 进行采样决策
	// 这样可以确保：
	// 1. 网关层决定采样后，下游服务会继承采样决策，不会重新采样
	// 2. 异步消息中的 trace context 也会继承采样决策
	// 3. 只有根 span（网关层）才会根据采样率进行采样决策
	sampler := tracesdk.ParentBased(tracesdk.TraceIDRatioBased(sampleRate))

	// 创建 TracerProvider，使用批量导出器，设置合理的缓冲区大小
	// 设置最大队列大小防止内存爆炸，即使端点不存在也不会无限增长
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp,
			tracesdk.WithBatchTimeout(5*time.Second), // 5秒批量导出
			tracesdk.WithMaxExportBatchSize(512),     // 最大批量大小
			tracesdk.WithMaxQueueSize(2048),          // 最大队列大小，防止内存爆炸
		),
		tracesdk.WithResource(res),
		tracesdk.WithSampler(sampler),
	)

	// 设置为全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracerProvider = tp
	tracer = tp.Tracer(cfg.ServiceName)

	return nil
}

// noopExporter 空导出器，用于追踪未启用或连接失败时
// 实现 tracesdk.SpanExporter 接口，防止内存泄漏
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []tracesdk.ReadOnlySpan) error {
	// 直接丢弃，不占用内存
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}

// Shutdown 关闭追踪器
func Shutdown(ctx context.Context) error {
	if tracerProvider != nil {
		return tracerProvider.Shutdown(ctx)
	}
	return nil
}

// GetTracer Tracer 获取追踪器
func GetTracer() trace.Tracer {
	if tracer == nil {
		return otel.Tracer("default")
	}
	return tracer
}

// StartSpan 开始一个新的 span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
}

// SpanFromContext 从 context 获取 span
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext 从 context 提取 TraceID
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanIDFromContext 从 context 提取 SpanID
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// extractHostPort 从 URL 中提取 host:port
// 支持格式: http://localhost:4318/v1/traces 或 localhost:4318
func extractHostPort(endpoint string) string {
	// 如果包含 http:// 或 https://，移除它们
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}
	// 移除路径部分
	if idx := strings.Index(endpoint, "/"); idx != -1 {
		endpoint = endpoint[:idx]
	}
	// 如果没有端口，添加默认端口 4318
	if !strings.Contains(endpoint, ":") {
		endpoint += ":4318"
	}
	return endpoint
}
