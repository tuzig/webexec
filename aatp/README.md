# Acceptance Test Procedure

TL:DR; From the project's root `./atps/test`

This folder contains automated acceptance test procedures for webexec. 
The tests are using docker-compose for lab setup and playwright
for end-to-end and user journies testing.

The script can also accept one of more argument with a folder name.
Each of these folders focus on a different aspect of users' scenarios.

## The runner

We use [playwright](https://playwright.dev) as the test runner and use
its syntax and expectations. To pass options to playwright use the 
`PWARGS` enviornment variable. I use it to get the tests to stop
after the first failure and keep the logs short:

```
PWARGS=-x ./qa/qa.bash fubar
```


Run `npx playwright test --help` for the list of options
