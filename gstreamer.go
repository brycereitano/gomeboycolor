// This simple test application serve live generated WebM content on webpage
// using HTML5 <video> element.
// The bitrate is low so you need to wait long for video if you browser has
// big input buffer.
package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"syscall"
	"time"
	"unsafe"

	"github.com/sergey789/gst"
	"github.com/ziutek/glib"
)

var frame uint64
var screen = make([]byte, size)

const size = uint64(160 * 144 * 3)

type Index struct {
	width, height int
}

var homeTemplate = template.Must(template.New("").Parse(`
<!doctype html>
<html>
	<head>
		<meta charset='utf-8'>
		<title>Live WebM video</title>
		<script>
		window.addEventListener("load", function(evt) {

				var output = document.getElementById("output");
				var input = document.getElementById("input");
				var ws = new WebSocket("{{.WebSocket}}");
				ws.onerror = function(evt) {
						console.log("ERROR: " + evt.data);
				}

				var buttons = document.querySelectorAll("button");
				for(var i=0; i<buttons.length; i++) {
					buttons[i].onclick = function(evt) {
							evt.preventDefault();
							if (!ws) { return }
							console.log("SEND: "+evt.target.id);
							ws.send(evt.target.id);
					};
				}
		});
		</script>
	</head>
	<body>
		<video src='/video' width={{.Width}} height={{.Height}} autoplay></video><br>
		<table>
		<form>
			<tr>
				<td></td>
				<td><button id="up">Up</button></td>
				<td></td>
				<td></td>
				<td><button id="start">Start</button></td>
			</tr>
			<tr>
				<td><button id="left">Left</button></td>
				<td><button id="down">Down</button></td>
				<td><button id="right">Right</button></td>
				<td></td>
				<td><button id="select">Select</button></td>
			</tr>
		</table>
	</body>
</html>`))

func (ix *Index) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	homeTemplate.Execute(wr, struct {
		WebSocket     string
		Width, Height int
	}{fmt.Sprintf("ws://%s/ws", req.Host), ix.width, ix.height})
}

type WebM struct {
	gbc    *GomeboyColor
	src    *gst.AppSrc
	pl     *gst.Pipeline
	sink   *gst.Element
	ticker *time.Ticker
	conns  map[int]net.Conn
}

func (wm *WebM) Play() {
	wm.pl.SetState(gst.STATE_PLAYING)
}

func (wm *WebM) Stop() {
	wm.pl.SetState(gst.STATE_READY)
}

func (wm *WebM) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	// Obtain fd
	conn, _, err := wr.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("http.Hijacker.Hijack:", err)
		return
	}
	file, err := conn.(*net.TCPConn).File()
	if err != nil {
		log.Println("net.TCPConn.File:", err)
		return
	}
	fd, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		log.Println("syscall.Dup:", err)
		return
	}
	// Send HTTP header
	_, err = io.WriteString(
		file,
		"HTTP/1.1 200 OK\r\n"+
			"Content-Type: video/webm\r\n\r\n",
	)
	if err != nil {
		log.Println("io.WriteString:", err)
		return
	}
	file.Close()

	// Save connection in map (workaround)
	wm.conns[fd] = conn

	// Pass fd to the multifdsink
	wm.sink.Emit("add", fd)
}

// Handler for connection closing
func (wm *WebM) cbClientFdRemoved(fd int32) {
	wm.conns[int(fd)].Close()
	syscall.Close(int(fd))
	delete(wm.conns, int(fd))
}

func (wm *WebM) cbNeedData(src *gst.AppSrc, _ uint32) {
	data := <-wm.gbc.io.ScreenOutputChannel
	//<-wm.ticker.C
	buf := gst.NewBufferAllocate(uint(size))
	for i := 0; i < int(size); i += 3 {
		var x = (i / 3) % 160
		var y = (i / 3) / 160
		screen[i] = data[y][x].Red
		screen[i+1] = data[y][x].Green
		screen[i+2] = data[y][x].Blue
	}
	buf.Fill(0, unsafe.Pointer(&screen[0]), uint(size))
	src.PushBuffer(buf)
}

func NewWebM(gbc *GomeboyColor, width, height, fps int) *WebM {
	wm := new(WebM)
	wm.ticker = time.NewTicker(time.Second / time.Duration(fps))
	wm.conns = make(map[int]net.Conn)
	wm.gbc = gbc

	wm.src = gst.NewAppSrc("Test source")
	wm.src.SetProperty("do-timestamp", true)

	enc1 := gst.ElementFactoryMake("videoconvert", "FFMPEG Color Space")
	enc2 := gst.ElementFactoryMake("vp8enc", "VP8 encoder")

	mux := gst.ElementFactoryMake("webmmux", "WebM muxer")
	mux.SetProperty("streamable", true)

	wm.sink = gst.ElementFactoryMake("multifdsink", "Multifd sink")
	wm.sink.SetProperty("sync", false)
	//wm.sink.SetProperty("recover-policy", 1) // keyframe
	//wm.sink.SetProperty("sync-method", 0)    // latest-keyframe

	wm.pl = gst.NewPipeline("WebM generator")
	wm.pl.Add(wm.src.AsElement(), enc1, enc2, mux, wm.sink)

	filter := gst.NewCapsSimple(
		"video/x-raw",
		glib.Params{
			"format":    "RGB",
			"width":     int32(width),
			"height":    int32(height),
			"framerate": &gst.Fraction{fps, 1},
		},
	)
	wm.src.LinkFiltered(enc1, filter)
	enc1.Link(enc2, mux, wm.sink)

	wm.sink.ConnectNoi("client-fd-removed", (*WebM).cbClientFdRemoved, wm)
	wm.src.Connect("need-data", (*WebM).cbNeedData, wm)

	return wm
}

func staticHandler(wr http.ResponseWriter, req *http.Request) {
	http.ServeFile(wr, req, req.URL.Path[1:])
}
