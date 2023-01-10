# hh-responder
hh-responder is cli tool for searching and applying vacancies on [Headhunter](https://hh.ru/) job aggregator.

Only host hh.ru is supported now.

## Building
clone the repo and build:
```
go build -ldflags="-X 'hh-responder/cmd.version=v1.0.1'"
```

## Usage
The one should somehow get an API access key from and set it as environment variable `HH_TOKEN` before using the tool. Please read 
[Oauth docs](https://api.hh.ru/openapi/en/redoc) for more information.

hh-responder uses [vacancies api](https://github.com/hhru/api/blob/master/docs_eng/vacancies.md#search) for searching based query parameters passed in configuration file

example of config file:
```yaml
search:
	# Documentation for search parameters - https://github.com/hhru/api/blob/master/docs_eng/vacancies.md
    clusters: false
    order_by: publication_time
    areas:
        - 28
        - 13
        - 1001
        - 74
        - 236
        - 146
        - 85
        - 199
        - 114
        - 5046
        - 94
    text: NAME:(SRE NOT Senior)
    schedules:
        - remote
        - fullDay
        - flexible
    period: 30
apply:
	# your resume title.
	resume: Middle SRE
	# your cover letter.
	message: "Hello, I'm interested in this vacancy. Please consider my resume."
	exclude:
		# exclude vacancies if a employer in the list.
        employers:
        # Test employer - https://hh.ru/employer/3331116
        - 3331116

```

Then the one can run the cli. For example on linux:
```
HH_TOKEN=$(cat ~/hh_token.txt) ./hh-responder run --config ./hh-responder-example.yaml
```

## To do list:
- Add GH actions
- Add tests
