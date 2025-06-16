package ips

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

type ipInfo struct {
	IP      string `json:"ip"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Org     string `json:"org"`
	Loc     string `json:"loc"`
}

type Server struct {
	httpClient  *http.Client
	ipCache     sync.Map
	logoImage   image.Image
	titleFont   font.Face
	bodyFont    font.Face
	smallFont   font.Face
	contextPool sync.Pool
	assets      embed.FS
}

func NewServer(assets embed.FS) *Server {
	s := &Server{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		assets:     assets,
		contextPool: sync.Pool{
			New: func() interface{} {
				return gg.NewContext(600, 200)
			},
		},
	}
	s.initFonts()
	return s
}

func (s *Server) initFonts() {
	tt, err := opentype.Parse(goregular.TTF)
	if err != nil {
		log.Printf("解析字体失败: %v", err)
		return
	}
	
	s.titleFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    20,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("创建标题字体失败: %v", err)
	}
	
	s.bodyFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    16,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("创建正文字体失败: %v", err)
	}
	
	s.smallFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    13,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("创建小字体失败: %v", err)
	}
}

func (s *Server) LoadLogo(logoPath string) {
	if logoPath != "" {
		file, err := os.Open(logoPath)
		if err != nil {
			log.Printf("无法打开外部logo文件: %v，使用内嵌logo", err)
			s.logoImage = s.loadEmbeddedLogo()
			return
		}
		defer file.Close()
		
		ext := strings.ToLower(filepath.Ext(logoPath))
		var img image.Image
		
		switch ext {
		case ".png":
			img, err = png.Decode(file)
		case ".jpg", ".jpeg":
			img, err = jpeg.Decode(file)
		default:
			log.Printf("不支持的图片格式: %s，使用内嵌logo", ext)
			s.logoImage = s.loadEmbeddedLogo()
			return
		}
		
		if err != nil {
			log.Printf("解码外部logo图片失败: %v，使用内嵌logo", err)
			s.logoImage = s.loadEmbeddedLogo()
			return
		}
		
		s.logoImage = resizeLogo(img, 64, 64)
		log.Printf("已加载外部logo: %s", logoPath)
	} else {
		s.logoImage = s.loadEmbeddedLogo()
	}
}

func (s *Server) loadEmbeddedLogo() image.Image {
	logoFiles := []string{"assets/logo.png", "assets/logo.jpg", "assets/logo.jpeg"}
	
	for _, logoFile := range logoFiles {
		data, err := s.assets.ReadFile(logoFile)
		if err != nil {
			continue
		}
		
		var img image.Image
		ext := strings.ToLower(filepath.Ext(logoFile))
		
		switch ext {
		case ".png":
			img, err = png.Decode(bytes.NewReader(data))
		case ".jpg", ".jpeg":
			img, err = jpeg.Decode(bytes.NewReader(data))
		}
		
		if err == nil {
			log.Printf("使用默认logo")
			return resizeLogo(img, 64, 64)
		}
	}
	
	log.Printf("未找到内嵌logo文件，使用默认logo")
	return createDefaultLogo()
}

func resizeLogo(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := x * bounds.Dx() / width
			srcY := y * bounds.Dy() / height
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}
	
	return dst
}

func createDefaultLogo() image.Image {
	size := 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	
	const cornerRadius = 12
	
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			ratio := float64(y) / float64(size)
			r := uint8(30 + ratio*40)
			g := uint8(60 + ratio*80)
			b := uint8(114 + ratio*100)
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	
	mask := image.NewAlpha(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var inCorner bool
			
			if x <= cornerRadius && y <= cornerRadius {
				dx := float64(x - cornerRadius)
				dy := float64(y - cornerRadius)
				inCorner = dx*dx + dy*dy <= float64(cornerRadius*cornerRadius)
			}
			if x >= size-cornerRadius && y <= cornerRadius {
				dx := float64(x - (size - cornerRadius))
				dy := float64(y - cornerRadius)
				inCorner = dx*dx + dy*dy <= float64(cornerRadius*cornerRadius)
			}
			if x <= cornerRadius && y >= size-cornerRadius {
				dx := float64(x - cornerRadius)
				dy := float64(y - (size - cornerRadius))
				inCorner = dx*dx + dy*dy <= float64(cornerRadius*cornerRadius)
			}
			if x >= size-cornerRadius && y >= size-cornerRadius {
				dx := float64(x - (size - cornerRadius))
				dy := float64(y - (size - cornerRadius))
				inCorner = dx*dx + dy*dy <= float64(cornerRadius*cornerRadius)
			}
			
			if (x > cornerRadius && x < size-cornerRadius) ||
			   (y > cornerRadius && y < size-cornerRadius) ||
			   inCorner {
				mask.Set(x, y, color.Alpha{255})
			}
		}
	}
	
	result := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if _, _, _, a := mask.At(x, y).RGBA(); a > 0 {
				result.Set(x, y, img.At(x, y))
			}
		}
	}
	
	return result
}

func (s *Server) getClientIP(c *gin.Context) string {
	// 检查各种代理头
	headers := []string{
		"CF-Connecting-IP",
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Client-IP",
		"X-Forwarded",
		"X-Cluster-Client-IP",
	}
	
	for _, header := range headers {
		if ip := c.GetHeader(header); ip != "" {
			ips := strings.Split(ip, ",")
			clientIP := strings.TrimSpace(ips[0])
			if net.ParseIP(clientIP) != nil {
				return clientIP
			}
		}
	}
	
	// 获取远程地址
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

func (s *Server) lookupGeo(ip string) string {
	if val, ok := s.ipCache.Load(ip); ok {
		return val.(string)
	}
	
	if isLocalIP(ip) {
		loc := "本地网络"
		s.ipCache.Store(ip, loc)
		return loc
	}
	
	url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "未知地区"
	}
	
	req.Header.Set("User-Agent", "iptracker/2.0")
	req.Header.Set("Accept", "application/json")
	
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("地理位置查询失败: %v", err)
		return "未知地区"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("地理位置查询返回状态码: %d", resp.StatusCode)
		return "未知地区"
	}

	var info ipInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.Printf("解析地理位置信息失败: %v", err)
		return "未知地区"
	}
	
	var parts []string
	if info.Country != "" {
		parts = append(parts, info.Country)
	}
	if info.Region != "" {
		parts = append(parts, info.Region)
	}
	if info.City != "" {
		parts = append(parts, info.City)
	}
	
	loc := strings.Join(parts, " ")
	if loc == "" {
		loc = "未知地区"
	}
	
	s.ipCache.Store(ip, loc)
	return loc
}

func isLocalIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	
	return parsedIP.IsLoopback() || parsedIP.IsPrivate() || parsedIP.IsLinkLocalUnicast()
}

func (s *Server) generateSVG(ip, ua, loc, now string) string {
	var logoBase64 string
	if s.logoImage != nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, s.logoImage); err == nil {
			logoBase64 = base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}
	
	if len(ua) > 70 {
		ua = ua[:67] + "..."
	}
	
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="200" viewBox="0 0 600 200">
  <defs>
    <linearGradient id="bg" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" style="stop-color:#1e293b;stop-opacity:1" />
      <stop offset="100%%" style="stop-color:#0f172a;stop-opacity:1" />
    </linearGradient>
    <filter id="shadow" x="-20%%" y="-20%%" width="140%%" height="140%%">
      <feDropShadow dx="0" dy="4" stdDeviation="8" flood-color="#000" flood-opacity="0.3"/>
    </filter>
  </defs>
  <rect width="100%%" height="100%%" fill="url(#bg)" rx="16" ry="16" filter="url(#shadow)"/>
  <image x="24" y="24" width="64" height="64" href="data:image/png;base64,%s"/>
  <text x="112" y="50" fill="#ffffff" font-family="system-ui, -apple-system, sans-serif" font-size="20" font-weight="600">您的IP: %s</text>
  <text x="112" y="78" fill="#cbd5e1" font-family="system-ui, -apple-system, sans-serif" font-size="16">时间: %s</text>
  <text x="112" y="106" fill="#cbd5e1" font-family="system-ui, -apple-system, sans-serif" font-size="16">地区: %s</text>
  <text x="24" y="150" fill="#94a3b8" font-family="system-ui, -apple-system, monospace" font-size="13">UA: %s</text>
  <circle cx="570" cy="30" r="8" fill="#10b981" opacity="0.8"/>
  <text x="550" y="35" fill="#10b981" font-family="system-ui, -apple-system, sans-serif" font-size="12" text-anchor="end">在线</text>
</svg>`,
		logoBase64,
		html.EscapeString(ip),
		html.EscapeString(now),
		html.EscapeString(loc),
		html.EscapeString(ua))
}

func (s *Server) generatePNG(ip, ua, loc, now string) []byte {
	dc := s.contextPool.Get().(*gg.Context)
	defer s.contextPool.Put(dc)
	
	dc.Clear()
	
	const width, height = 600, 200
	const cornerRadius = 16
	
	gradient := gg.NewLinearGradient(0, 0, width, height)
	gradient.AddColorStop(0, color.RGBA{30, 41, 59, 255})
	gradient.AddColorStop(1, color.RGBA{15, 23, 42, 255})
	dc.SetFillStyle(gradient)
	dc.DrawRoundedRectangle(0, 0, width, height, cornerRadius)
	dc.Fill()
	
	dc.SetRGBA(1, 1, 1, 0.1)
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(0, 0, width, height, cornerRadius)
	dc.Stroke()

	if s.logoImage != nil {
		dc.DrawImageAnchored(s.logoImage, 56, 56, 0.5, 0.5)
	}

	if s.titleFont != nil && s.bodyFont != nil && s.smallFont != nil {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		dc.SetFontFace(s.titleFont)
		dc.DrawString(fmt.Sprintf("您的IP: %s", ip), 112, 45)
		
		dc.SetColor(color.RGBA{203, 213, 225, 255})
		dc.SetFontFace(s.bodyFont)
		dc.DrawString(fmt.Sprintf("时间: %s", now), 112, 75)
		dc.DrawString(fmt.Sprintf("地区: %s", loc), 112, 100)
		
		dc.SetColor(color.RGBA{148, 163, 184, 255})
		dc.SetFontFace(s.smallFont)
		if len(ua) > 70 {
			ua = ua[:67] + "..."
		}
		dc.DrawString(fmt.Sprintf("UA: %s", ua), 24, 160)
		
		dc.SetColor(color.RGBA{16, 185, 129, 255})
		dc.DrawCircle(570, 30, 8)
		dc.Fill()
		
		dc.SetFontFace(s.smallFont)
		dc.DrawStringAnchored("在线", 550, 30, 1, 0.5)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		log.Printf("PNG 编码失败: %v", err)
		return nil
	}
	return buf.Bytes()
}

func (s *Server) ipImageHandler(c *gin.Context) {
	ip := s.getClientIP(c)
	ua := c.Request.UserAgent()
	if ua == "" {
		ua = "未知浏览器"
	}
	
	loc := s.lookupGeo(ip)
	now := time.Now().Format("2006-01-02 15:04:05")

	path := c.Request.URL.Path
	var isSVG bool
	
	if strings.HasSuffix(path, ".svg") {
		isSVG = true
	} else if strings.HasSuffix(path, ".png") {
		isSVG = false
	} else {
		accept := c.GetHeader("Accept")
		isSVG = !strings.Contains(accept, "image/png") || strings.Contains(accept, "image/svg+xml")
	}

	c.Header("Cache-Control", "public, max-age=60")
	c.Header("ETag", fmt.Sprintf(`"%s-%d"`, ip, time.Now().Unix()/60))

	if isSVG {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusOK, s.generateSVG(ip, ua, loc, now))
	} else {
		pngData := s.generatePNG(ip, ua, loc, now)
		if pngData == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成图像失败"})
			return
		}
		c.Header("Content-Type", "image/png")
		c.Data(http.StatusOK, "image/png", pngData)
	}
}

func (s *Server) Run(port string) error {
	gin.SetMode(gin.ReleaseMode)
	
	r := gin.New()
	
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Accept, Content-Type")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		
		c.Next()
	})
	
	api := r.Group("/api")
	{
		api.GET("/ip", s.ipImageHandler)
		api.GET("/ip.png", s.ipImageHandler)
		api.GET("/ip.svg", s.ipImageHandler)
	}
	
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "github.com/sky22333",
			"usage": map[string]string{
				"default": "GET /api/ip (默认 SVG 格式)",
				"png":     "GET /api/ip.png (PNG 格式)",
				"svg":     "GET /api/ip.svg (SVG 格式)",
			},
		})
	})

	serverPort := ":" + port
	log.Printf(" API 访问链接:")
	log.Printf("   • http://localhost%s/api/ip (默认 SVG 格式)", serverPort)
	log.Printf("   • http://localhost%s/api/ip.png (PNG 格式)", serverPort)
	log.Printf("   • http://localhost%s/api/ip.svg (SVG 格式)", serverPort)
	
	return r.Run(serverPort)
}
