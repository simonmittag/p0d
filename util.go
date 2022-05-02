package p0d

import (
	"fmt"
	. "github.com/logrusorgru/aurora"
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
		Cyan("\n  -===============-.").String()+Red("      .=.    .-.      ").String()+Cyan("-============--.  ").String()+
			Cyan("\n.======---======-").String()+Red("    .    ===   -==.").String()+Cyan("       .==============- ").String()+
			Cyan("\n=====    .=====.").String()+Red("    ==-.  -==  -==   .---.").String()+Cyan("   -====.   .====-").String()+
			Cyan("\n====-    -====").String()+Red("       -==-  -=  --  .===-.").String()+Cyan("     -===-    -====").String()+
			Cyan("\n====-    ====-").String()+Red("  ...    .--  -  -  --.").String()+Cyan("          ====    -====").String()+
			Cyan("\n=====    ====").String()+Red("   .-===-.. ....-").String()+Yellow("_").String()+Red("-... ..-=====.").String()+Cyan("  .===    -====").String()+
			Cyan("\n.============").String()+Red("       ...... =").String()+Yellow("//|\\\\").String()+Red("........").String()+Cyan("       ===    -====").String()+
			Cyan("\n  -==========").String()+Red("     ........ -").String()+Yellow("\\\\|//").String()+Red(".     .-----.").String()+Cyan("  ===    -====").String()+
			Cyan("\n         ====.").String()+Red("  -====--.  .. -").String()+Yellow("‾").String()+Red("-. --   ..-==.").String()+Cyan("  -===    -====").String()+
			Cyan("\n         =====").String()+Red("         .--. .=  =  .==.").String()+Cyan("        ====    -====").String()+
			Cyan("\n         -====-").String()+Red("     .-==-  .=-  ==   -==-").String()+Cyan("     ====-    -====").String()+
			Cyan("\n          =====-").String()+Red("   .--.   .==.  ===   .--").String()+Cyan("   .=====.   .=====").String()+
			Cyan("\n          -======-.").String()+Red("       ==-   -==").String()+Cyan("        -===============.").String()+
			Cyan("\n           --------.").String()+Red("      ..     -.").String()+Cyan("       ---============-  ").String()+
			"\n\n")
}
