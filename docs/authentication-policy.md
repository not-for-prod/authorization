# Authentication policy example

A policy describes which combinations of verified methods complete an
authentication transaction. The engine selects a policy by tenant and intent;
for example, tenant `acme` and intent `sign_in`.

The [annotated YAML example](examples/sign-in-policy.yaml) illustrates one policy
body. For Go constructor wiring and policies across multiple tenants and
intents, see [Authentication engine configuration](authentication-configuration.md)
and the [complete policy YAML example](examples/auth-engine.yaml). A policy
loader and public facade for other Go modules have not yet been implemented.
The internal application constructor accepts a host-supplied policy repository.

```yaml
all:
  - id: primary
    any:
      - method: password
      - method: email_code
      - method: oauth
        providers:
          - google
          - github

  - id: mfa
    any:
      - method: totp
      - method: telegram_code
```

This policy requires one primary proof **and** one MFA proof:

```text
(password OR email_code OR oauth/google OR oauth/github)
AND
(totp OR telegram_code)
```

## Reading the fields

| Field | Meaning |
| --- | --- |
| `all` | Every child requirement must be satisfied. |
| `any` | At least one child requirement must be satisfied. |
| `id` | Configurable requirement identifier used to associate evidence with the requirement it satisfies. Names such as `primary` and `mfa` have no built-in meaning. |
| `method` | Authentication mechanism, such as `password`, `oauth`, or `totp`. |
| `providers` | Allowed providers for this method requirement. Here, Google or GitHub can satisfy the OAuth option. Methods without a provider omit this field. |

## Example outcomes

Each proof below is assumed to have been successfully verified and attached to
the indicated requirement in the same transaction.

| Evidence | Outcome |
| --- | --- |
| Password for `primary` | `mfa` remains unsatisfied. |
| Google OAuth for `primary`, TOTP for `mfa` | Policy satisfied. |
| Email code for `primary`, Telegram code for `mfa` | Policy satisfied. |
| Google OAuth and GitHub OAuth, both for `primary` | `mfa` remains unsatisfied. |
| OAuth through a provider absent from `providers` | Does not satisfy the OAuth option. |

Evidence is scoped to its requirement. If you later allow `email_code` in both
groups, evidence recorded for `primary` does not automatically satisfy `mfa`.

## Runtime behavior

The YAML describes acceptable combinations, not a fixed sequence. The engine
evaluates the remaining requirements and offers methods that are currently
available. For example, TOTP may become available after the user is identified
and their TOTP enrollment is known; Telegram code may require a linked Telegram
account.

An unavailable method does not remove a requirement. If neither MFA option can
be used, this policy cannot be completed with the currently available methods.

After the policy is satisfied, the engine can finalize the transaction into a
principal and ask the session manager to create a session. Authentication
methods produce evidence; they do not create sessions themselves.

The host application configures provider clients and session implementations
in Go and supplies them through the authentication service constructor. This
policy tree only names the methods and providers allowed for the intent.
