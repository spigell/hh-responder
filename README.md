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

## AI Assistance

Set the `ai.enabled` flag in the configuration file to let hh-responder evaluate vacancies against the selected resume and generate tailored cover letters with Google's Gemini API. Supply the credentials via the `ai.gemini.api-key-file` field or the `GEMINI_API_KEY_FILE` environment variable. You can tune the filtering aggressiveness with `ai.minimum-fit-score` (0 disables the score threshold) and control retry attempts on transient or short quota errors via `ai.gemini.max-retries`. See `hh-responder-example.yaml` for a complete example.

## To do list:
- Add GH actions
- Add tests
