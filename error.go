package p0d

const refused string = "connection refused"
const reset string = "connection reset by peer"
const closed string = "use of closed network connection"
const eof string = "EOF"
const broken string = "broken pipe"
const dialtimeout string = "i/o timeout"

var connectionErrors = map[string]string{
	refused:     refused,
	reset:       reset,
	closed:      closed,
	eof:         eof,
	broken:      broken,
	dialtimeout: dialtimeout,
}
