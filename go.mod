module github.com/bnema/dumber

go 1.25.3

require github.com/rs/zerolog v1.33.0

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

replace (
	github.com/bnema/puregotk-webkit => /home/brice/projects/puregotk-webkit
	github.com/jwijenbergh/puregotk => /home/brice/projects/puregotk
)
