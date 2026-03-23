package metrics

//go:generate $GOPATH/bin/mockgen -destination=prometheus_mock.go -package=metrics github.com/prometheus/client_golang/prometheus Registerer
