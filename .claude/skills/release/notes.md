# Review the release notes

The facts come from the repository (`scripts/release-notes.sh`); the prose comes from a model, and a program checked
only its form. It cannot tell whether a line is true or names the right PR, and PR text is untrusted input. Fill in
what the draft's comments ask for, then check each section:

- Keep every section, and write "None" in one with nothing to say: an empty section and a missing one mean different
  things to an operator.
- A patch release is compared with the previous minor version: remove what an earlier patch of its minor version
  already announced.
- **Upgrading**: add how long each data upgrade step takes on a large instance, and anything to do by hand. When this
  release removes data that an earlier release carried over, name that release: operators must have upgraded through
  it. Check it with the new release's `my schema apply -dry-run` on a database of the previous release.
- **Breaking changes**: say what each item means for an operator or a client, whether a schema change removes data
  and which release carried it over, and which `!` commits matter outside the repository. ActivityPub formats and
  federated identifiers never change in a release; never list them.
- **API changes**: the client API grows by addition. An operation may be removed only when a release of an earlier
  minor version shipped it `deprecated: true` and its Sunset date has passed; the draft marks in bold a removal that
  breaks this, and it must not ship. Name the replacement of a deprecated operation its description does not name,
  and the reason for a change made without the deprecation period (a security fix only). A new deprecation's Sunset
  falls after the next planned minor release.
- **mykit, mybot**: what each change marked `!` and each deprecated or removed operation means for a library's user.
- When the facts failed, the sections ask to be written by hand and there is no prose: write them from what changed
  since the previous release in `api/openapi.yaml`, `dataUpgrades`, `retiredKeys`, and the configuration reference.
