all:
	@@echo "building out_pulsar.so ..."
	go build -buildmode=c-shared -o out_pulsar.so main.go 2>/dev/null
	@@echo "DONE"

clean:
	rm -rf *.so *.h

