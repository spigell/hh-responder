# hh-responder
hh-responder is cli tool for searching and applying vacancies on [Headhunter](https://hh.ru/) job aggregator.

Only host hh.ru is supported now.

## Building
clone the repo and build:
```
go build -ldflags="-X 'hh-responder/cmd.version=v1.0.1'"
```

## Usage

One should somehow get an API access key, store it in a file, and point hh-responder to that file via the `token-file` configuration setting or the `HH_TOKEN_FILE` environment variable before using the tool. Storing the token directly in the configuration or other environment variables is not supported. Please read (Oauth docs)[https://api.hh.ru/openapi/en/redoc) for more information.

hh-responder uses [vacancies API](https://github.com/hhru/api/blob/master/docs_eng/vacancies.md#search) for searching based query parameters passed in a configuration file
For the example of the config file please see here - [hh-responder-example.yaml](hh-responder-example.yaml)

You can optionally override the default HTTP User-Agent header sent to hh.ru by setting the `user-agent` field in the configuration file.

Then one can run the CLI. For example on Linux:
```
./hh-responder run --config ./hh-responder-example.yaml
```

## To do list:
- Add GH actions
- Add tests
