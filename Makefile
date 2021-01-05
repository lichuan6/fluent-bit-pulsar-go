all:
	go build -buildmode=c-shared -o out_pulsar.so main.go

clean:
	rm -rf *.so *.h

