package app

var cmdUsage = `
usage: %[1]s [-flags] URL URL URL ...

  flags:

  -version        display version information (%s)
  -verbose        verbose output (default: false)

`

var (
	optFlagVersion *bool
	optFlagVerbose *bool
)
