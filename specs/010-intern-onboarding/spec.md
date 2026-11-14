# Feature Specification: a manager adds a new intern

**Feature Branch**: `feat/intern-onboarding`

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: request of the team, 14.11: new interns are added by their manager and can sign in. Case rules:
authorization and roles already exist at Avito, no own HR system (`docs/avito/case.md`); permissions are RBAC
plus ABAC, checked on the server (`docs/design/system-design.md` §8); company systems are mocks with a protobuf
contract (§9.2).

## Summary

A manager opens «Команда», presses «Добавить стажёра» and fills a short form. The company's account system
(the `auth-roles` mock) creates the account and the directory entry and returns a one-time password, shown to the
manager once. The intern signs in with it on the usual login page; buddy creates the intern's own data (plan
progress, threads) on the first sign-in. Buddy never stores the password.

## Who may add

- **Role**: `manager` only. Interns, mentors and HR roles get `PERMISSION_DENIED` (buddy API) before any call.
- **Attribute**: the new intern always joins the manager's own team; there is no team argument. The mentor must be
  a person of that team. Both are checked again in the mock: it trusts the requester id, not buddy's screen.

## Contract

### auth-roles mock: `CreateAccount`

- **In**: `requester_id`, `display_name`, `login`, `direction` (`backend|frontend|qa`), `mentor_id` (person id),
  `start_date` (`YYYY-MM-DD`, empty means today).
- **Out**: `account` (as `GetAccount`) and `temporary_password`.
- **Effects**: the account can sign in at once; the directory (`people` mock) lists the intern in the team with
  relations mentor, manager (the requester's person) and the team's HR BP.
- **Errors**: `PERMISSION_DENIED` (requester is not a manager), `INVALID_ARGUMENT` (empty name, login not matching
  `^[a-z][a-z0-9._-]{2,31}$`, unknown direction, mentor outside the team, bad date), `ALREADY_EXISTS` (login taken).
- **Password**: 12 characters from an alphabet without look-alikes, from `crypto/rand`; the mock keeps only a
  bcrypt hash. Created accounts survive a restart when `MOCKS_STATE_FILE` is set.

### buddy API: `POST /v1/private/team/interns`

- **In**: `{display_name, login, direction, mentor_id, start_date?}`.
- **Out**: `{user, login, temporary_password}`; the response is not logged and not stored.
- **Errors**: the mock's codes pass through; without `MOCKS_ADDR` it is `UNIMPLEMENTED` (no account system).

### First sign-in

- Login checks the account in `auth-roles` as before. If buddy has no user row for it yet, it creates one from the
  account (role, team, direction, start date). The plan is the shared plan with every item «впереди».

## Acceptance

1. Manager adds «Вера Новикова», login `vnovikova`, backend, mentor Анна Белова: the response has the account and a
   12-character password; the team list shows Вера within a minute (directory cache).
2. `vnovikova` signs in with that password: «Сегодня» shows day and stage, the plan has every item, the chat works.
3. Intern or mentor calling the endpoint: 403, nothing created.
4. Login taken: 409 with a message; name empty or mentor from another team: 400.
5. The same login after a mock restart with `MOCKS_STATE_FILE`: still signs in.
