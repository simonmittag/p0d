package p0d

const refused string = "connection refused"
const reset string = "connection reset by peer"
const closed string = "use of closed network connection"
const eof string = "EOF"
const broken string = "broken pipe"
const dialtimeout string = "i/o timeout"
const buffer string = "no buffer space available"

const read string = "read"
const write string = "write"
const connection string = "connection"

var errorMapping = map[string]string{
	read:        read,
	write:       write,
	connection:  connection,
	refused:     connection,
	reset:       connection,
	closed:      connection,
	eof:         read,
	broken:      connection,
	dialtimeout: connection,
	buffer:      read,
}
