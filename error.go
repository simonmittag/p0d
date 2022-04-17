package p0d

const refused string = "connection refused"
const reset string = "connection reset by peer"

var connectionErrors = map[string]string{refused: refused, reset: reset}
