package p0d

import (
	"fmt"
	. "github.com/logrusorgru/aurora"
	"github.com/shirou/gopsutil/mem"
	"net"
	"strconv"
	"syscall"
	"time"
)

func logv(args ...any) {
	log("%v", args...)
}

func log(s string, args ...any) {
	fmt.Printf(timefmt(s), args...)
}

func slog(s string, args ...any) {
	time.Sleep(time.Millisecond * 125)
	fmt.Printf(timefmt(s), args...)
}

func timefmt(s string) string {
	now := time.Now().Format(time.Kitchen)
	return fmt.Sprintf("%s %s\n", Gray(8, now), s)
}

func getUlimit() (string, int64) {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		return "not determined", 0
	} else {
		return fmt.Sprintf("%s",
				Yellow(FGroup(int64(rLimit.Cur)))),
			int64(rLimit.Cur)
	}
}

func getRAMBytes() uint64 {
	v, _ := mem.VirtualMemory()
	return v.Total
}

func FGroup(n int64) string {
	in := strconv.FormatInt(n, 10)
	numOfDigits := len(in)
	if n < 0 {
		numOfDigits-- // First character is the - sign (not a digit)
	}
	numOfCommas := (numOfDigits - 1) / 3

	out := make([]byte, len(in)+numOfCommas)
	if n < 0 {
		in, out[0] = in[1:], '-'
	}

	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]
		if i == 0 {
			return string(out)
		}
		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func PrintLogo() (int, error) {
	return fmt.Printf("%v",
		Cyan("\n        .╬╠╠╠╠╠╠╠╠╠╬`").String()+Red("     ╠╠  ,╬╠     ").String()+Cyan("╙╠╠╠╠╠╠╠╠╠φ").String()+
			Cyan("\n        ╠╠╠╙   ╠╠╠╙").String()+Red("   ╠╬ε └╠ε ╠╩  ╔#▒  ").String()+Cyan("`╠╠╠   ╙╠╠╠").String()+
			Cyan("\n        ╠╠╠   ]╠╠╩").String()+Red("     └╝▒ ╙Γ ╩ ╔╬╩`     ").String()+Cyan("╠╠ε   ╠╠╠").String()+
			Cyan("\n        ╠╠╠ε  ╚╠╠").String()+Red("   ╝╠▒╗, \"╓δΘ╦\",╓φ▒╬╠▒ε ").String()+Cyan("╚╠▒   ╠╠╠").String()+
			Cyan("\n         ╚╠╠╠╠╠╠╠").String()+Red("         ,╠ ").String()+Yellow("⬤ ").String()+Red(" ╩≈╔╓╓╓,  ").String()+Cyan("]╠╠   ╠╠╠").String()+
			Cyan("\n              ╠╠╠⌐").String()+Red(" ²╠╠╝╩\"²╓∩⌠╙╠`φ, `╙╝╠  ").String()+Cyan("╠╠╠   ╠╠╠").String()+
			Cyan("\n              [╠╠╠ ").String()+Red("     ╓@╬  ╬ ╠ε ╙╠φ   ").String()+Cyan(",╠╠Γ   ╠╠╠").String()+
			Cyan("\n               ╠╠╠▒").String()+Red("  '╝╩\" .╬╠ ]╠ε  ╚╩  ").String()+Cyan("╔╠╠╠   ╓╠╠╠").String()+
			Cyan("\n               ╚╠╠╠╠φ,").String()+Red("    ╠╩   ╠╛     ").String()+Cyan("φ╠╠╠╠╠╠╠╠╠╩").String()+
			"\n")
}

func PrintVersion() {
	fmt.Printf(Cyan("p0d %s\n").String(), Version)
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

type SpinnerAnim struct {
	chars []string
	index int
}

func NewSpinnerAnim() *SpinnerAnim {
	return &SpinnerAnim{
		chars: []string{"\\", "|", "/", "-"},
		index: 0,
	}
}

func (b *SpinnerAnim) Next() string {
	c := b.chars[b.index]
	b.index = (b.index + 1) % len(b.chars)
	return c
}

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
