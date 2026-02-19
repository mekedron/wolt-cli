from pydantic import BaseModel


class HashableModel(BaseModel):
    def __hash__(self) -> int:
        return hash(self.model_dump_json())
