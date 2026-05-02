from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class ChangeEntry(BaseModel):
    kind: str
    path: str


class ReportPayload(BaseModel):
    agent_id: UUID
    agent_name: str
    scan_path: str
    timestamp: datetime
    total_files: int = Field(ge=0)
    num_created: int = Field(ge=0)
    num_modified: int = Field(ge=0)
    num_deleted: int = Field(ge=0)
    changes: list[ChangeEntry] = Field(default=[], max_length=20_000)


class ReportResp(BaseModel):
    pass
