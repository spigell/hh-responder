# hh-responder
hh-responder is cli tool for searching and applying vacancies on [Headhunter](https://hh.ru/) job aggregator.

Only host hh.ru is supported now.

## Building
clone the repo and build:
```
go build -ldflags="-X 'hh-responder/cmd.version=v1.0.1'"
```

## Usage

One should somehow get an API access key and set it as environment variable `HH_TOKEN` before using the tool. Please read (Oauth docs)[https://api.hh.ru/openapi/en/redoc) for more information.

hh-responder uses [vacancies API](https://github.com/hhru/api/blob/master/docs_eng/vacancies.md#search) for searching based query parameters passed in a configuration file
For the example of the config file please see here - [hh-responder-example.yaml](hh-responder-example.yaml)

Then one can run the CLI. For example on Linux:
```
HH_TOKEN=$(cat ~/hh_token.txt) ./hh-responder run --config ./hh-responder-example.yaml
```

## To do list:
- Add GH actions
- Add tests
