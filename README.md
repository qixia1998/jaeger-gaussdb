# Jaeger-GaussDB

![GitHub License](https://img.shields.io/github/license/qixia1998/jaeger-gaussdb)
![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=flat&logo=go&logoColor=white)
![Kubernetes](https://img.shields.io/badge/kubernetes-%23326ce5.svg?style=flat&logo=kubernetes&logoColor=white)

jaeger-gaussdb is a GaussDB backed storage solution .

## Run
You can use this command.



```
1. git clone https://github.com/qixia1998/jaeger-gaussdb.git
2. cd jaeger-gaussdb
3. go mod tidy
4. sql generate
```
Then: Performing a Database Migration at GaussDB.
* use Goose
* manual execution

Next:

<!-- x-release-please-start-version -->
```
go run cmd/jaeger-gaussdb/main.go --database.url='postgresql://db_user:db_password@host:port/jaeger' --grpc-server.host-port=jaeger-gaussdb:12345 --log-level=debug

```
<!-- x-release-please-end -->

```
# database connection options
database:
    # url to the database
    url: "postgresql://db_user:db_password@host:port/jaeger" 
    
    # the maximum number of database connections 
    maxConns: 10 
```

## Usage
You can start jaeger in docker with the following command, including jaeger-ui。


> docker run \                                                                                               
-e SPAN_STORAGE_TYPE=grpc \
-e GRPC_STORAGE_SERVER=jaeger-gaussdb:12345 \
-e COLLECTOR_ZIPKIN_HOST_PORT=:9411 \
-p 6831:6831/udp \
-p 6832:6832/udp \
-p 5778:5778 \  
-p 16686:16686 \
-p 4317:4317 \
-p 4318:4318 \    
-p 14250:14250 \  
-p 14268:14268 \
-p 14269:14269 \
-p 9411:9411 \
jaegertracing/all-in-one:1.62.0



`--grpc-storage.server=jaeger-postgresql:12345`

`SPAN_STORAGE_TYPE="grpc"`

You can use **main.go** in the `example` directory to batch create spans and send trace data to Jaeger.

The official jaeger documentation is the best place to look for detailed instructions on using a external storage plugin. https://www.jaegertracing.io/docs/1.63/deployment/#storage-plugin


## Legacy
This project started out as a simple fork of [Jozef Slezak's plugin of the same name](jozef-slezak/jaeger-postgresql), but was eventually completely rewritten. 
