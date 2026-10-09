package utils

import (
	"fmt"

	"github.com/cheggaaa/pb/v3"
)

// NoProgress 为真时不渲染进度条（systemd/cron 等非交互环境里进度条会污染日志）
var NoProgress = false

type Bar struct {
	pb *pb.ProgressBar
}

func NewBar(count int, MyStrStart, MyStrEnd string) *Bar {
	if NoProgress {
		return &Bar{}
	}
	tmpl := fmt.Sprintf(`{{counters . }} {{ bar . "[" "-" (cycle . "↖" "↗" "↘" "↙" ) "_" "]"}} %s {{string . "MyStr" | green}} %s `, MyStrStart, MyStrEnd)
	bar := pb.ProgressBarTemplate(tmpl).Start(count)
	return &Bar{pb: bar}
}

func (b *Bar) Grow(num int, MyStrVal string) {
	if b.pb == nil {
		return
	}
	b.pb.Set("MyStr", MyStrVal).Add(num)
}

func (b *Bar) Done() {
	if b.pb == nil {
		return
	}
	b.pb.Finish()
}
