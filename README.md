# Multiplexer: HTTP-Server & URL Crawler

```
Приложение представляет собой http-сервер с одним хендлером.
Хендлер на вход получает POST-запрос со списком url в json-формате.
Сервер запрашивает данные по всем этим url и возвращает результат клиенту в json-формате.
Если в процессе обработки хотя бы одного из url получена ошибка, обработка всего списка прекращается и клиенту возвращается текстовая ошибка.

Ограничения:
- для реализации задачи следует использовать Go 1.13 или выше
- использовать можно только компоненты стандартной библиотеки Go
- сервер не принимает запрос если количество url в в нем больше 20
- сервер не обслуживает больше чем 100 одновременных входящих http-запросов
- для каждого входящего запроса должно быть не больше 4 одновременных исходящих
- таймаут на запрос одного url - секунда
- обработка запроса может быть отменена клиентом в любой момент, это должно повлечь за собой остановку всех операций связанных с этим запросом
- сервис должен поддерживать 'graceful shutdown'
```

## Run a Server

```Bash
$ go run ./cmd/multiplexer/main.go

> 2021/10/28 17:21:24 app started on port: 80
```

## Request Validation

### POST-Method

```Bash
$ curl http://localhost/crawler

> method not allowed: expected "POST": got "GET"
```

### JSON Input

```Bash
$ curl -X POST http://localhost/crawler

> unsupported "Content-Type" header: expected "application/json": got ""
```

### Empty Body

```Bash
$ curl -X POST http://localhost/crawler -H "Content-Type: application/json" 

> bad request: empty request body
```

### Input Contract Compliance

```Bash
$ curl -X POST http://localhost/crawler -d "some random data" \
    -H "Content-Type: application/json"

> bad request: invalid character 's' looking for beginning of value
```

```Bash
$ curl -X POST http://localhost/crawler -d '{"some":"field"}' \
    -H "Content-Type: application/json"

> bad request: no URLs passed
```

```Bash
$ curl -X POST http://localhost/crawler \
    -H "Content-Type: application/json" \
    -d '{"urls":[
      "1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
      "11", "12", "13", "14", "15", "16", "17", "18", "19", 
      "20", "21", "22"]}'

> max number of URLs exceeded: 22 of 20"
```

```Bash
$ curl -X POST http://localhost/crawler \
    -H "Content-Type: application/json" \
    -d '{"urls":["some random text"]}'

> invalid url: "some random text"
```

## Error Handling

### HTTP Status Code Check

```Bash
$ curl -X POST http://localhost/crawler \
    -H "Content-Type: application/json" \
    -d '{"urls":["https://httpstat.us/500"]}'

> failed to crawl "https://httpstat.us/500": unexpected response status code: 500
```

### Request Timeout

```Bash
$ curl -X POST http://localhost/crawler \
    -H "Content-Type: application/json" \
    -d '{"urls":["https://httpstat.us/200?sleep=5000"]}'

> failed to crawl "https://httpstat.us/200?sleep=5000": 
  failed to send a request: Get "https://httpstat.us/200?sleep=5000": 
  context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

## Exit Fast & Context Cancel

```Bash
2021/10/28 16:53:13 app started on port: 80
2021/10/28 16:53:19 crawler: received 3 tasks: validating URL format
2021/10/28 16:53:19 crawler: starting 3 workers
2021/10/28 16:53:19 crawler: sending request: http://google.com
2021/10/28 16:53:19 crawler: sending request: http://yandex.ru
2021/10/28 16:53:19 crawler: sending request: http://69.63.176.13
2021/10/28 16:53:19 crawler: worker stopped: no more tasks
2021/10/28 16:53:19 crawler: unmarshal response body to JSON: invalid character '<' looking for beginning of value
2021/10/28 16:53:19 crawler: worker stopped: no more tasks
2021/10/28 16:53:19 crawler: error occurred: stopping other goroutines
2021/10/28 16:53:19 crawler: send request: Get "http://69.63.176.13": context canceled
2021/10/28 16:53:19 crawler: worker stopped: context canceled
2021/10/28 16:53:19 crawler: send request: Get "http://www.google.com/": context canceled
2021/10/28 16:53:19 crawler: error occurred: skipping new results
2021/10/28 16:53:19 crawler: error occurred: skipping new results
2021/10/28 16:53:19 crawler: worker stopped: context canceled
2021/10/28 16:53:19 crawler: results channel closed
2021/10/28 16:53:19 crawler: exit with error: failed to crawl "http://yandex.ru": unmarshal response body to JSON: invalid character '<' looking for beginning of value
2021/10/28 16:53:19 handler: failed to crawl "http://yandex.ru": unmarshal response body to JSON: invalid character '<' looking for beginning of value
```

## Graceful Shutdown

```Bash
2021/10/28 16:53:13 app started on port: 80
...
^C2021/10/28 16:56:44 OS signal received: interrupt
2021/10/28 16:56:44 http: setting graceful timeout: 3.00s
2021/10/28 16:56:44 http: awaiting traffic to stop: 3.00s
2021/10/28 16:56:44 http: shutting down: disabling keep-alive
2021/10/28 16:56:44 closer: http: shutting down: context deadline exceeded

Process finished with exit code 0
```

## Limited Number of Simultaneous Incoming Requests

The problem is solved with a simple buffered-channel window. 
Before new connection can be established, it has to acquire a lock 
(get queued to a channel). When connection is closed a lock is released.

## Limited Number of Outgoing Requests

The problem can be solved in multiple ways, e.g. having a fixed number 
of workers that evenly pull tasks from the queue, or we can just run 
a goroutine per URL and try to acquire a spot in the buffered-channel 
window to limit the number of concurrently running tasks.

An unlimited number of goroutines (due to unknown size of the URLs array) 
can end up with waste of resources and crash afterwards. That's why I chose 
a worker-pool solution, it solves this exact problem just fine.

## Happy Path

```Bash
$ curl -X POST http://localhost/crawler -H "Content-Type: application/json" -d '{"urls":[ 
    "https://jsonplaceholder.typicode.com/todos/1", 
    "https://jsonplaceholder.typicode.com/todos/2", 
    "https://jsonplaceholder.typicode.com/todos/3", 
    "https://jsonplaceholder.typicode.com/todos/4",
    "https://jsonplaceholder.typicode.com/todos/5", 
    "https://jsonplaceholder.typicode.com/todos/6", 
    "https://jsonplaceholder.typicode.com/todos/7", 
    "https://jsonplaceholder.typicode.com/todos/8", 
    "https://jsonplaceholder.typicode.com/todos/9", 
    "https://jsonplaceholder.typicode.com/todos/10", 
    "https://jsonplaceholder.typicode.com/todos/11", 
    "https://jsonplaceholder.typicode.com/todos/12", 
    "https://jsonplaceholder.typicode.com/todos/13", 
    "https://jsonplaceholder.typicode.com/todos/14", 
    "https://jsonplaceholder.typicode.com/todos/15", 
    "https://jsonplaceholder.typicode.com/todos/16", 
    "https://jsonplaceholder.typicode.com/todos/17", 
    "https://jsonplaceholder.typicode.com/todos/18", 
    "https://jsonplaceholder.typicode.com/todos/19", 
    "https://jsonplaceholder.typicode.com/todos/20" 
]}'

> {
  "results": [
    {
      "url": "https://jsonplaceholder.typicode.com/todos/3",
      "response": {
        "code": 200,
        "body": {
          "userId": 1,
          "id": 3,
          "title": "fugiat veniam minus",
          "completed": false
        }
      }
    },
    ...
    {
      "url": "https://jsonplaceholder.typicode.com/todos/20",
      "response": {
        "code": 200,
        "body": {
          "userId": 1,
          "id": 20,
          "title": "ullam nobis libero sapiente ad optio sint",
          "completed": true
        }
      }
    }
  ]
}
```

