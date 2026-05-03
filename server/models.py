from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class ChangeEntry(BaseModel):
    kind: str = Field(max_length=10)  # "created", "modified", "deleted"
    path: str = Field(max_length=4096)  # Reasonable max path length


class ReportPayload(BaseModel):
    agent_id: UUID
    agent_name: str = Field(max_length=255)
    scan_path: str = Field(max_length=4096)
    timestamp: datetime
    total_files: int = Field(ge=0)
    num_created: int = Field(ge=0)
    num_modified: int = Field(ge=0)
    num_deleted: int = Field(ge=0)
    changes: list[ChangeEntry] = Field(default=[], max_length=20_000)


class ReportResp(BaseModel):
    pass
