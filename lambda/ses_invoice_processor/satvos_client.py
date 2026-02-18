"""HTTP client for the SATVOS backend API."""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timezone

import requests

from .exceptions import AuthenticationError, SatvosAPIError

logger = logging.getLogger("ses_invoice_processor")


@dataclass
class TokenPair:
    access_token: str
    refresh_token: str
    expires_at: datetime


@dataclass
class ProcessingResult:
    collection_id: str
    collection_name: str
    files_uploaded: int
    files_failed: list[str] = field(default_factory=list)
    documents_created: int = 0
    documents_failed: list[str] = field(default_factory=list)


class SatvosClient:
    """Client for interacting with the SATVOS backend API."""

    def __init__(self, base_url: str, tenant_slug: str = "passpl"):
        self._base_url = base_url
        self._tenant_slug = tenant_slug
        self._session = requests.Session()
        self._token: TokenPair | None = None

    def authenticate(self, email: str, password: str) -> None:
        """Authenticate with the SATVOS API and store tokens.

        Raises AuthenticationError on failure.
        """
        resp = self._session.post(
            f"{self._base_url}/auth/login",
            json={"tenant_slug": self._tenant_slug, "email": email, "password": password},
        )
        if resp.status_code != 200:
            raise AuthenticationError(
                f"Login failed: {resp.status_code}",
                status_code=resp.status_code,
                response_body=resp.text,
            )

        data = resp.json()["data"]
        self._token = TokenPair(
            access_token=data["access_token"],
            refresh_token=data["refresh_token"],
            expires_at=datetime.fromisoformat(data["expires_at"]),
        )
        self._session.headers["Authorization"] = f"Bearer {self._token.access_token}"

    def _ensure_auth(self) -> None:
        """Refresh the access token if it's expired or about to expire (60s buffer)."""
        if self._token is None:
            raise AuthenticationError("Not authenticated — call authenticate() first")

        now = datetime.now(timezone.utc)
        expires = self._token.expires_at
        if expires.tzinfo is None:
            expires = expires.replace(tzinfo=timezone.utc)

        buffer_seconds = 60
        if (expires - now).total_seconds() > buffer_seconds:
            return

        resp = self._session.post(
            f"{self._base_url}/auth/refresh",
            json={"refresh_token": self._token.refresh_token},
        )
        if resp.status_code != 200:
            raise AuthenticationError(
                f"Token refresh failed: {resp.status_code}",
                status_code=resp.status_code,
                response_body=resp.text,
            )

        data = resp.json()["data"]
        self._token = TokenPair(
            access_token=data["access_token"],
            refresh_token=data["refresh_token"],
            expires_at=datetime.fromisoformat(data["expires_at"]),
        )
        self._session.headers["Authorization"] = f"Bearer {self._token.access_token}"

    def create_collection(self, name: str, description: str, *, owner_email: str | None = None) -> str:
        """Create a collection and return its ID.

        If owner_email is provided, the backend will look up the user by email
        within the tenant and grant them owner permission on the collection.

        Raises SatvosAPIError on failure.
        """
        self._ensure_auth()
        body: dict = {"name": name, "description": description}
        if owner_email:
            body["owner_email"] = owner_email
        resp = self._session.post(
            f"{self._base_url}/collections",
            json=body,
        )
        if resp.status_code not in (200, 201):
            raise SatvosAPIError(
                f"Failed to create collection: {resp.status_code}",
                status_code=resp.status_code,
                response_body=resp.text,
            )
        return resp.json()["data"]["id"]

    def batch_upload_files(self, collection_id: str, attachments: list) -> list[dict]:
        """Upload files to a collection via multipart batch upload.

        Returns the per-file results array.
        Raises SatvosAPIError on total failure.
        """
        self._ensure_auth()
        files = [
            ("files", (att.filename, att.data, att.content_type))
            for att in attachments
        ]
        # Remove Content-Type header so requests sets multipart boundary
        headers = {k: v for k, v in self._session.headers.items() if k.lower() != "content-type"}
        resp = self._session.post(
            f"{self._base_url}/collections/{collection_id}/files",
            files=files,
            headers=headers,
        )
        if resp.status_code not in (200, 201, 207):
            raise SatvosAPIError(
                f"Batch upload failed: {resp.status_code}",
                status_code=resp.status_code,
                response_body=resp.text,
            )
        return resp.json()["data"]

    def create_document(self, file_id: str, collection_id: str) -> str:
        """Create a document (triggers async LLM parsing) and return its ID.

        Raises SatvosAPIError on failure.
        """
        self._ensure_auth()
        resp = self._session.post(
            f"{self._base_url}/documents",
            json={
                "file_id": file_id,
                "collection_id": collection_id,
                "document_type": "invoice",
                "parse_mode": "single",
            },
        )
        if resp.status_code not in (200, 201):
            raise SatvosAPIError(
                f"Failed to create document: {resp.status_code}",
                status_code=resp.status_code,
                response_body=resp.text,
            )
        return resp.json()["data"]["id"]

    def process_attachments(
        self, company_name: str, attachments: list, *, sender_email: str | None = None
    ) -> ProcessingResult:
        """Orchestrate the full pipeline: create collection, upload files, create documents.

        Continues on partial failures and records them in the result.
        """
        now = datetime.now(timezone.utc)
        collection_name = f"{company_name} - {now.strftime('%Y-%m-%d %H:%M')}"
        if sender_email:
            description = f"Auto-imported from email sent by {sender_email}"
        else:
            description = f"Auto-imported from email for {company_name}"

        collection_id = self.create_collection(collection_name, description, owner_email=sender_email)
        logger.info("Created collection %s: %s", collection_id, collection_name)

        result = ProcessingResult(
            collection_id=collection_id,
            collection_name=collection_name,
            files_uploaded=0,
        )

        # Batch upload all files
        upload_results = self.batch_upload_files(collection_id, attachments)

        # Create documents for each successfully uploaded file
        for item in upload_results:
            if not item.get("success", False):
                error_msg = item.get("error", "unknown error")
                filename = item.get("file", {}).get("original_name", "unknown")
                result.files_failed.append(f"{filename}: {error_msg}")
                logger.warning("File upload failed: %s — %s", filename, error_msg)
                continue

            result.files_uploaded += 1
            file_id = item["file"]["id"]
            filename = item["file"].get("original_name", "unknown")

            try:
                doc_id = self.create_document(file_id, collection_id)
                result.documents_created += 1
                logger.info("Created document %s for file %s (%s)", doc_id, file_id, filename)
            except SatvosAPIError as exc:
                result.documents_failed.append(f"{filename}: {exc}")
                logger.warning("Document creation failed for file %s: %s", filename, exc)

        return result
