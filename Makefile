build::
	cd src; GOOS=linux GOARCH=amd64 go build -o bootstrap scraper.go
	zip -j ./infrastructure/scraper.zip ./src/bootstrap