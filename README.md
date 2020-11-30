![image](https://user-images.githubusercontent.com/47/100630683-7916a880-32f0-11eb-8252-e88cee41c432.png)

## Usage

```yaml
name: sentry-actions
on:
  workflow_run:
    # Sample completed workflows
    types: [completed]
    # List the names of the workflows you'd like to sample
    workflows:
      - docker
      - test
      - fail
jobs:
  sentry-actions:
    runs-on: ubuntu-latest
    steps:
      - uses: urcomputeringpal/sentry-actions@main
        with:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          SENTRY_DSN: ${{ secrets.SENTRY_DSN }}
          SENTRY_RELEASE: ${{ github.sha }}
          SENTRY_ENVIRONMENT: production

```