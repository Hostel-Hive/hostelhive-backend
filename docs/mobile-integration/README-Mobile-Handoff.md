# HostelHive mobile integration handoff

Prepared: 15 October 2026.

## Start here

1. Open HostelHive-Mobile-API-Integration-Guide-2026-10-15.html in a browser.
   It contains a clickable contents list,25 actual endpoints, schemas/examples,
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
- Postman collection:25 requests, no automatic execution scripts or real credentials.
- This README.

## Important availability limits

The guide describes the current backend working tree, not proof of deployment.
Issue89 push code is locally implemented on the feature branch in this snapshot; confirm
its server availability, migration25 and FCM configuration with the backend owner.
Live phone delivery is not yet verified. Full Student profile, compulsory first-login
state, Going Home, quota, cancellation and bulk mark-read remain explicit gaps.
Do not show successful mock behavior as if those APIs already exist.

No backend code, database records or Firebase settings were changed to create
this handoff. The package contains documentation and request definitions only.
