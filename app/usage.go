package app

var cmdUsage = `
usage: %[1]s [-flags] URL URL URL ...

  flags:

  -version        display version information (%s)
  -verbose        verbose output / debug logging (default: false)
  -chunks N       chunk size for parallel download (default: 5)
  -limit RATE     bandwidth limit, e.g. 5M, 500K (default: 0, unlimited)
  -output DIR     output directory (default: current directory)

`
