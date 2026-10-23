# HostelHive mobile integration handoff

> Student self-service editing is available through
> [PATCH /api/v1/me/student-profile](../student-self-profile-editing.md) for both
> mobile and web clients. Older packaged HTML/Postman/PDF files do not include it;
> use the current Markdown contract and generated Swagger.

> Policy #118: use [the current movement contract](../hostel-movement-policy.md).
> Older packaged HTML/Postman/PDF handoffs describe superseded leave approval and
> allowance behavior; use the current Markdown guide and generated Swagger instead.

Updated: 19 October 2026.

## Start here

1. Open HostelHive-Mobile-API-Integration-Guide-2026-10-15.html in a browser.
   It contains a clickable contents list,30 actual endpoints, schemas/examples,
   authentication, retries, image rules, phone lifecycle and integration checks.
2. Start with the 12-feature checklist, then read sections1,2 and13 before adapting the Flutter repositories. The earlier
   proposed /auth, /passes and /inbox paths are not the actual Go API contract.
3. Obtain the real reachable API base URL and its running revision from the backend
   owner. No verified public staging URL or real credentials are bundled here.
4. Import HostelHive-Mobile-API-2026-10-15.postman_collection.json in Postman.
   Follow guide section15. Use a dedicated Student account. Token/UUID variables
   are blank placeholders; set private local values without exporting real secrets.
5. Use the Markdown guide as the editable/source-control copy. It contains the
   same contract as the HTML copy. The HTML has inline styling and no remote assets.

## Included files

- HTML guide: browser-readable handoff, printable if desired.
- Markdown guide: editable developer documentation.
- Postman collection:30 requests, no automatic execution scripts or real credentials.
- This README.

## Important availability limits

The guide describes the current backend working tree, not proof of deployment.
Issue89 push code is locally implemented on the feature branch in this snapshot; confirm
its server availability, migration25 and FCM configuration with the backend owner.
Live phone delivery is not yet verified. Issue #91 adds owned Student profile, accommodation, presence and image reads; see section4.3 and confirm deployment. Issue #93 adds owned recorded attendance history; see section4.4. Issue95 adds invitation setup state and completion; real email/Flutter acceptance remains pending. Issue100 adds Going Home declarations (section6.5), with migration27. Quota, cancellation and bulk mark-read remain explicit gaps.
Do not show successful mock behavior as if those APIs already exist.

No backend code, database records or Firebase settings were changed to create
this handoff. The package contains documentation and request definitions only.

## Student onboarding (#95)

University email is the login identifier with a separate HostelHive password; no
university SSO or university-password collection. Admin invitations let Students
choose their password on the Firebase-hosted page. Read guide section3.5 for fresh
sign-in, /me setup_required and empty POST /me/setup/complete. Pending setup blocks
all other protected calls, including push. The Postman collection now contains30
Student requests. Admin APIs/CSV are documented in ../student-invitations.md.
Apply migration26 and confirm server configuration/deployment. Automated backend
checks do not certify live email receipt or Flutter integration.
