module github.com/simonmittag/p0d

go 1.18

require (
	github.com/ghodss/yaml v1.0.0
	github.com/google/uuid v1.3.0
	github.com/gosuri/uilive v0.0.4
	github.com/hako/durafmt v0.0.0-20210608085754-5c1018a4e16b
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/simonmittag/procspy v0.0.0-20191119070947-e8cf3f846a67
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2
)

require (
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20210208195552-ff826a37aa15 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/showwin/speedtest-go v1.1.5 // indirect
	github.com/weaveworks/procspy v0.0.0-20150706124340-cb970aa190c3 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220412211240-33da011f77ad // indirect
	golang.org/x/text v0.3.6 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/gosuri/uilive v0.0.4 => github.com/mrnugget/uilive v0.0.4-fix-escape
