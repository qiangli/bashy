---
id: 73fa4d6ccc81
kind: feature
title: S601.2 Google Docs, Sheets & Drive Export helper (examples/google/docs_sheets.py)
seq: 3200
status: todo
priority: p1
created: 2026-11-09T09:04:53.815429Z
sprint: 601
sprint_id: 3f93eee1-fb25-5828-a779-966c345ff576
sprint_title: Google Workspace custom command MVP (examples/google)
---

Create standalone helper module examples/google/docs_sheets.py and hermetic unit test suite examples/google/test_docs_sheets.py (do not modify google.bsh or other files). Implement CLI/callable functions wrapping gws (via GWS_BIN or gws on PATH with GWS_ACCOUNT and GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE preflight and SECRET_VIA_ARGV refusal):
1. drive-export <file-id> mimeType=<target-mime> [output=<path>]: exports Google Docs/Sheets/Slides via 'gws drive files export --params {"fileId":...,"mimeType":...}' supporting text/markdown, text/csv, text/plain, and application/pdf.
2. sheets-get <spreadsheet-id> [range=Sheet1!A1:Z100]: reads values via 'gws sheets spreadsheets values get --params {"spreadsheetId":...,"range":...} --format=json'.
3. sheets-append <spreadsheet-id> range=Sheet1!A1 values:=JSON [--dry-run]: appends rows via 'gws sheets spreadsheets values append --params {"spreadsheetId":...,"range":...,"valueInputOption":"USER_ENTERED"} --json {"values":...} --format=json'.
4. docs-get <document-id>: fetches a Google Doc via 'gws docs documents get --params {"documentId":...} --format=json' and includes a helper to extract plain text from body.content structural elements.
Verify with: python3 -m unittest examples/google/test_docs_sheets.py -v
