package cmd

import (
	"embed"
	"fmt"
	"io"
	iofs "io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"goqlprinter/api"
	icfg "goqlprinter/internal/config"
	"goqlprinter/internal/logging"
	isvc "goqlprinter/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// EmbeddedFiles must be set from main package before Execute().
var EmbeddedFiles embed.FS

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	startupMsg := `
  ____        _      __  __       _ _
 |  _ \      | |    |  \/  |     (_) |
 | |_) | __ _| |__  | \  / | __ _ _| | ___ _ __
 |  _ < / _' | '_ \ | |\/| |/ _' | | |/ _ \ '_ \
 | |_) | (_| | |_) || |  | | (_| | | |  __/ | | |
 |____/ \__,_|_.__(_)_|  |_|\__,_|_|_|\___|_| |_|
`
	fmt.Println(startupMsg)

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}
	logging.Init(logLevel)

	slog.Info("Loading configuration...")
	cfg, err := icfg.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	Cfg = cfg

	slog.Info("Server configuration loaded successfully",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"backend", cfg.App.Backend,
		"default_printer", cfg.App.DefaultPrinter,
		"font_dirs", cfg.App.FontDirs)

	BackendProvider = InitBackendProvider(cfg)

	ps := isvc.NewPrinterService(BackendProvider)
	ps.InitializeDefaultPrinter(cfg.App.DefaultPrinter)

	fs := isvc.NewFontService(cfg.App.FontDirs)
	handlers := api.NewHandlers(ps, fs, cfg)
	sseHub := api.NewSSEHub(ps)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(os.Getenv("GIN_MODE"))
	}

	switch {
	case strings.EqualFold(logLevel, "DEBUG"):
		gin.DefaultWriter = os.Stdout
		gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
			slog.Debug("route registered", "method", httpMethod, "path", absolutePath, "handler", handlerName)
		}
	case strings.EqualFold(logLevel, "ERROR"):
		gin.DefaultWriter = io.Discard
	default:
		gin.DefaultWriter = os.Stdout
	}

	slog.Info("Logging configured successfully")
	slog.Info("Using font directories from config", "font_dirs", cfg.App.FontDirs)

	scheme := "http"
	if cfg.Server.TLS {
		scheme = "https"
	}
	fmt.Printf("Brother printer driver is running. Open in browser:\n%s://%s:%d\n",
		scheme, cfg.Server.Host, cfg.Server.Port)

	r := gin.New()
	r.MaxMultipartMemory = 10 << 20 // 10 MB
	if logLevel != "ERROR" {
		r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[%s] %s %s %d %s %q\n",
				param.TimeStamp.Format("2006-01-02 15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
				param.Latency,
				param.ErrorMessage,
			)
		}))
	}
	r.Use(gin.Recovery())

	r.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL("doc.json"),
		ginSwagger.DefaultModelsExpandDepth(-1),
	))

	apiRoutes := r.Group("/api")
	if cfg.Server.Token != "" {
		apiRoutes.Use(api.TokenAuth(cfg.Server.Token))
		slog.Info("API token authentication enabled")
	}
	{
		apiRoutes.GET("/config", handlers.GetConfig)
		apiRoutes.GET("/printers", handlers.GetPrinters)
		apiRoutes.GET("/label-sizes", handlers.GetLabelSizes)
		apiRoutes.GET("/label-sizes/:id", handlers.GetLabelSize)
		apiRoutes.POST("/print", handlers.PrintLabel)
		apiRoutes.POST("/print_png", handlers.PrintPNGLabel)
		apiRoutes.POST("/print_png_raw", handlers.PrintPNGRaw)
		apiRoutes.POST("/print_qr", handlers.PrintQR)
		apiRoutes.POST("/print_svg", handlers.PrintSVG)
		apiRoutes.POST("/preview", handlers.PreviewLabel)
		apiRoutes.GET("/fonts", handlers.GetFonts)
		apiRoutes.POST("/status", handlers.GetStatus)
		apiRoutes.GET("/events", sseHub.HandleSSE)

		testRoutes := apiRoutes.Group("/test")
		{
			testRoutes.POST("/invalidate", handlers.TestInvalidate)
			testRoutes.POST("/initialize", handlers.TestInitialize)
			testRoutes.POST("/feed", handlers.TestFeed)
			testRoutes.POST("/set_media_and_feed", handlers.TestSetMediaAndFeed)
		}
	}

	distFS, err := iofs.Sub(EmbeddedFiles, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem for dist: %w", err)
	}

	assetsFS, err := iofs.Sub(distFS, "assets")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem for assets: %w", err)
	}

	r.StaticFS("/assets", http.FS(assetsFS))

	r.NoRoute(func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.RequestURI, "/api") {
			c.FileFromFS("/", http.FS(distFS))
		}
	})

	listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.TLS {
		slog.Info("Starting HTTPS server", "addr", listenAddr)
		return r.RunTLS(listenAddr, cfg.Server.CertFile, cfg.Server.KeyFile)
	}
	return r.Run(listenAddr)
}
