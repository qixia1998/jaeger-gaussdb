package main

import (
	"context"
	sql2 "database/sql"
	"fmt"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"io/fs"
	"jaeger-gaussdb/internal/logger"
	"jaeger-gaussdb/internal/sql"
	"jaeger-gaussdb/internal/store"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"google.golang.org/grpc"
)

// ProvideLogger returns a function that provides a logger
func ProvideLogger() any {
	return func(cfg Config) (*slog.Logger, error) {
		return logger.New(&cfg.LogLevel)
	}
}

func ProvideSqlDB() any {
	return func(cfg Config, logger *slog.Logger, lc fx.Lifecycle) (*sql2.DB, error) {
		databaseURL := cfg.Database.URL
		if databaseURL == "" {
			return nil, fmt.Errorf("invalid database url")
		}

		//err := sql.Migrate(logger, databaseURL)
		//if err != nil {
		//	return nil, fmt.Errorf("failed to migrate database: %w", err)
		//}

		db, err := sql2.Open("opengauss", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to open database connection: %w", err)
		}

		// 设置最大打开连接数
		maxOpenConns := 20
		if cfg.Database.MaxConns > 0 {
			maxOpenConns = cfg.Database.MaxConns
		}
		db.SetMaxOpenConns(maxOpenConns)

		// 设置最大空闲连接数
		maxIdleConns := 5
		if cfg.Database.MaxConns > 0 {
			maxIdleConns = cfg.Database.MaxConns
		}
		db.SetMaxIdleConns(maxIdleConns)

		// 设置连接超时时间
		connectTimeoutDuration := 10 * time.Second
		db.SetConnMaxLifetime(connectTimeoutDuration)

		// 验证连接
		ctx, cancelFn := context.WithTimeout(context.Background(), connectTimeoutDuration)
		defer cancelFn()

		err = db.PingContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}

		logger.Info("connected to gaussdb")

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				db.Close()
				return nil
			},
		})

		return db, nil
	}
}

// ProvideSpanStoreReader returns a function that provides a spanstore reader.
func ProvideSpanStoreReader() any {
	return func(pool *sql2.DB, logger *slog.Logger) spanstore.Reader {
		q := sql.New(pool)
		return store.NewInstrumentedReader(store.NewReader(q, logger), logger)
	}
}

// ProvideSpanStoreWriter returns a function that provides a spanstore writer
func ProvideSpanStoreWriter() any {
	return func(pool *sql2.DB, logger *slog.Logger) spanstore.Writer {
		q := sql.New(pool)
		return store.NewInstrumentedWriter(store.NewWriter(q, logger), logger)
	}
}

// ProvideDependencyStoreReader provides a dependencystore reader
func ProvideDependencyStoreReader() any {
	return func(pool *sql2.DB, logger *slog.Logger) dependencystore.Reader {
		q := sql.New(pool)
		return store.NewReader(q, logger)
	}
}

// ProvideHandler provides a grpc handler.
func ProvideHandler() any {
	return func(reader spanstore.Reader, writer spanstore.Writer, dependencyReader dependencystore.Reader) *shared.GRPCHandler {
		handler := shared.NewGRPCHandler(&shared.GRPCHandlerStorageImpl{
			SpanReader:          func() spanstore.Reader { return reader },
			SpanWriter:          func() spanstore.Writer { return writer },
			DependencyReader:    func() dependencystore.Reader { return dependencyReader },
			ArchiveSpanReader:   func() spanstore.Reader { return nil },
			ArchiveSpanWriter:   func() spanstore.Writer { return nil },
			StreamingSpanWriter: func() spanstore.Writer { return nil },
		})

		return handler
	}
}

// ProvideGRPCServer provides a grpc server.
func ProvideGRPCServer() any {
	return func(lc fx.Lifecycle, cfg Config, logger *slog.Logger) (*grpc.Server, error) {
		srv := grpc.NewServer()

		if cfg.GRPCServer.HostPort == "" {
			return nil, fmt.Errorf("invalid grpc-server.host-port given: %s", cfg.GRPCServer.HostPort)
		}

		lis, err := net.Listen("tcp", cfg.GRPCServer.HostPort)
		if err != nil {
			logger.Error("failed to listen on grpc server", "addr", cfg.GRPCServer.HostPort, "err", err)
			return nil, fmt.Errorf("failed to listen: %w", err)
		}

		logger.Info("grpc server started", "addr", cfg.GRPCServer.HostPort)

		lc.Append(fx.StartStopHook(
			func(ctx context.Context) error {
				go srv.Serve(lis)
				return nil
			},

			func(ctx context.Context) error {
				srv.GracefulStop()
				return lis.Close()
			},
		))

		return srv, nil
	}
}

func ProvideHealthServer() any {
	return func(lc fx.Lifecycle, logger *slog.Logger, cfg Config, srv *grpc.Server) *health.Server {
		hls := health.NewServer()

		// Register the health service with the gRPC server
		//grpc_health_v1.RegisterHealthServer(srv, hls)

		// Set the initial status to "SERVING"
		hls.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

		// 如果配置了健康检查服务的监听地址，则启动一个单独的 gRPC 服务器
		if cfg.HealthServer.HostPort != "" {
			lis, err := net.Listen("tcp", cfg.HealthServer.HostPort)
			if err != nil {
				logger.Error("failed to listen on health server", "addr", cfg.HealthServer.HostPort, "err", err)
				return nil
			}

			logger.Info("health server started", "addr", cfg.HealthServer.HostPort)

			lc.Append(fx.StartStopHook(
				func(ctx context.Context) error {
					go srv.Serve(lis)
					return nil
				},
				func(ctx context.Context) error {
					srv.GracefulStop()
					return lis.Close()
				},
			))
		}

		lc.Append(fx.StopHook(func(ctx context.Context) error {
			hls.Shutdown()
			return nil
		}))

		return hls
	}
}

// ProvideAdminServer provides the admin http server.
func ProvideAdminServer() any {
	return func(lc fx.Lifecycle, cfg Config, logger *slog.Logger) (*http.ServeMux, error) {
		mux := http.NewServeMux()

		srv := http.Server{
			Handler: mux,
		}

		if cfg.Admin.HTTP.HostPort == "" {
			return nil, fmt.Errorf("invalid admin.http.host-port given: %s", cfg.Admin.HTTP.HostPort)
		}

		lis, err := net.Listen("tcp", cfg.Admin.HTTP.HostPort)
		if err != nil {
			logger.Error("failed to listen on admin server", "addr", cfg.Admin.HTTP.HostPort, "err", err)
			return nil, fmt.Errorf("failed to listen: %w", err)
		}

		logger.Info("admin server started", "addr", lis.Addr())

		lc.Append(fx.StartStopHook(
			func(ctx context.Context) error {
				go srv.Serve(lis)
				return nil
			},

			func(ctx context.Context) error {
				return srv.Shutdown(ctx)
			},
		))

		return mux, nil
	}
}

var (
	spansTableDiskSizeGuage = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "jaeger_gaussdb",
		Name:      "spans_table_bytes",
		Help:      "The size of the spans table in bytes",
	})

	spansGuage = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "jaeger_gaussdb",
		Name:      "spans_count",
		Help:      "The number of spans",
	})
)

// Config is the configuration struct for the jaeger-gaussdb service.
type Config struct {
	Database struct {
		URL      string `mapstructure:"url"`
		MaxConns int    `mapstructure:"max-conns"`
	} `mapstructure:"database"`

	LogLevel string `mapstructure:"log-level"`

	GRPCServer struct {
		HostPort string `mapstructure:"host-port"`
	} `mapstructure:"grpc-server"`

	HealthServer struct {
		HostPort string `mapstructure:"host-port"`
	} `mapstructure:"health-server"`

	Admin struct {
		HTTP struct {
			HostPort string `mapstructure:"host-port"`
		}
	}
}

func ProvideConfig() func() (Config, error) {
	return func() (Config, error) {
		pflag.String("database.url", "", "the gaussdb connection url to use to connect to the database")
		pflag.Int("database.max-conns", 20, "Max number of database connections of which the plugin will try to maintain at any given time")
		pflag.String("log-level", "warn", "Minimal allowed log level")
		pflag.String("grpc-server.host-port", ":12345", "the host:port (eg 127.0.0.1:12345 or :12345) of the storage provider's gRPC server")
		pflag.String("admin.http.host-port", ":12346", "The host:port (e.g. 127.0.0.1:12346 or :12346) for the admin server, including health check, /metrics, etc.")

		v := viper.New()
		v.SetEnvPrefix("JAEGER_GAUSSDB")
		v.AutomaticEnv()
		v.SetConfigFile("jaeger-gaussdb")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
		pflag.Parse()
		v.BindPFlags(pflag.CommandLine)

		var cfg Config
		if err := v.ReadInConfig(); err != nil {
			_, ok := err.(*fs.PathError)
			_, ok2 := err.(viper.ConfigFileNotFoundError)

			if !ok && !ok2 {
				return cfg, fmt.Errorf("failed to read in config: %w", err)
			}
		}

		err := v.Unmarshal(&cfg)
		if err != nil {
			return cfg, fmt.Errorf("failed to decode configuration: %w", err)
		}

		return cfg, nil
	}
}

func main() {
	fx.New(
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger.With("component", "uber/fx")}
		}),
		fx.Provide(
			ProvideConfig(),
			ProvideLogger(),
			ProvideSqlDB(),
			ProvideSpanStoreReader(),
			ProvideSpanStoreWriter(),
			ProvideDependencyStoreReader(),
			ProvideHandler(),
			ProvideGRPCServer(),
			ProvideHealthServer(),
			ProvideAdminServer(),
		),
		fx.Invoke(func(srv *grpc.Server, hls *health.Server, handler *shared.GRPCHandler) error {
			return handler.Register(srv, hls)
		}),
		fx.Invoke(func(conn *sql2.DB, logger *slog.Logger, lc fx.Lifecycle) {
			ctx, cancelFn := context.WithCancel(context.Background())
			lc.Append(fx.StopHook(cancelFn))

			go func() {
				q := sql.New(conn)
				ticker := time.NewTicker(time.Second * 5)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Second*5)
						defer cancel()

						byteCount, err := q.GetSpansDiskSize(ctxWithTimeout)
						if err != nil {
							logger.Error("failed to query for disk size", "err", err)
							continue
						} else {
							spansTableDiskSizeGuage.Set(float64(byteCount))
						}

						count, err := q.GetSpansCount(ctxWithTimeout)
						if err != nil {
							logger.Error("failed to query for span count", "err", err)
							continue
						} else {
							spansGuage.Set(float64(count))
						}
					}
				}
			}()
		}),
		fx.Invoke(func(mux *http.ServeMux, conn *sql2.DB) {
			mux.Handle("/metrics", promhttp.Handler())
			mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancelFn := context.WithTimeout(r.Context(), time.Second*5)
				defer cancelFn()

				err := conn.PingContext(ctx)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}

				w.WriteHeader(http.StatusOK)
			}))
		}),
	).Run()
}
