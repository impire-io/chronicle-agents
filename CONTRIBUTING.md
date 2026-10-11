# Contributing to chronicle-agents

This repository is the code of chronicle-agents: humans and autonomous
agents collaborating in rooms kept as instances in a
[chronicle](https://github.com/impire-io/chronicle) store. It is a
solution built *on* chronicle, never a part of it (chronicle-hq decision
0048). Design lives in `chronicle-agents-hq`; capabilities land here
through its build handoff, never by invention in a pull request. A
contribution that changes a contract starts as a conversation there.

## The Developer Certificate of Origin

Contributions are accepted under the
[Developer Certificate of Origin, version 1.1](https://developercertificate.org/).
By adding a `Signed-off-by` trailer to a commit you certify that you wrote
the change or otherwise have the right to submit it under this
repository's license, and that you understand the contribution is public
and will be redistributed. Sign every commit:

```sh
git commit -s
```

The trailer must name you and a real address — `Signed-off-by: Ada
Lovelace <ada@example.com>`. Continuous integration refuses a pull
request whose commits lack it. There is no contributor license
agreement and no copyright assignment.

## The quality gate

`make check` — format, tidy, build, race-detected tests, lint — is green
before a change is done. Anything that speaks to chronicle is tested
against a real chronicle on a real embedded NATS server; mocking the
client is not accepted. Every addition names the present need it serves.

## License

The repository's license is the file `LICENSE`, the Sustainable Use
License. Contributions are licensed under it as it stands when they are
merged.
