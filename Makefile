project = tantrum
projectpath = ${PWD}
glidepath = ${PWD}/vendor/github.com/Masterminds/glide
redispath = ${PWD}/vendor/github.com/simongui/redis
wrkpath = ${PWD}/vendor/github.com/wg/wrk
wrk2path = ${PWD}/vendor/github.com/giltene/wrk2

target:
	@go build

test:
	@go test

integration: test
	@go test -tags=integration

$(glidepath)/glide:
	git clone https://github.com/Masterminds/glide.git $(glidepath)
	cd $(glidepath);make build
	cp $(glidepath)/glide .

$(redispath)/src/redis-benchmark:
	git clone https://github.com/simongui/redis.git $(redispath)
	cd $(redispath);make
	cp $(redispath)/src/redis-benchmark .

benchmark/wrk:
	if [ ! -d "$(wrkpath)" ]; then git clone https://github.com/wg/wrk.git $(wrkpath); fi
	cd $(wrkpath);make
	cp $(wrkpath)/wrk ./benchmark/wrk

benchmark/wrk2:
	if [ ! -d "$(wrk2path)" ]; then git clone https://github.com/giltene/wrk2.git $(wrk2path); fi
	cd $(wrk2path);make
	cp $(wrk2path)/wrk ./benchmark/wrk2

deps: $(glidepath)/glide $(redispath)/src/redis-benchmark benchmark/wrk benchmark/wrk2
	$(glidepath)/glide install
