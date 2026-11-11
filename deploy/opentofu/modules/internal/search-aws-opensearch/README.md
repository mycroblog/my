# search-aws-opensearch

Search on a domain of Amazon OpenSearch Service, `search = opensearch` with requests signed by the tasks' role. It reads a domain that the operator created, asks for the HTTP methods the server sends to it, and sets the settings of the engine. It never creates or changes the domain. It follows [the backend contract](../README.md#the-backend-contract). [Search engines](../../../../../docs/operations/search-engines.md#opensearch) describes the engine.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. Unverified:

- that the data source finds a domain and its endpoint as the tests assume, for a domain with a public endpoint and for one in a VPC;
- that the server reaches a domain in a VPC through the security groups given here;
- that an identity-based policy of the tasks' role is enough for a domain whose access policy lets the account in.

`mycroblog-aws` calls it for its section `search`.

| Input | Default | Meaning |
| --- | --- | --- |
| `domain_name` | required | The domain, in the region of the provider. It must exist before the plan |
| `index` | null | `search_opensearch_index`, the alias. Null keeps the server's default, `my-statuses` |
| `security_group_ids` | empty | Groups the tasks join to reach a domain in the VPC, which the domain's security group admits |

| Output | Value |
| --- | --- |
| `env` | `MY_SEARCH`, `MY_SEARCH_OPENSEARCH_URL` (the domain's endpoint), `MY_SEARCH_OPENSEARCH_AWS_SIGNING` (`es`), `MY_SEARCH_OPENSEARCH_AWS_REGION` (the domain's), and `MY_SEARCH_OPENSEARCH_INDEX` when `index` is set |
| `secret_env` | Empty. The requests are signed with the tasks' role |
| `permissions` | `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut`, and `es:ESHttpDelete` on `<domain ARN>/*` |
| `security_group_ids` | The input |
| `private_network` | The VPC of a domain in one, which must be the app's, or null |

What stays the operator's: the domain itself, its access policy, which must not deny the tasks' role, and with fine-grained access control a mapping of that role to a role of OpenSearch that may manage the indexes of the alias.
