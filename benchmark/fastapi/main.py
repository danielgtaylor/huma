from fastapi import FastAPI, Header, Depends, Response
from pydantic import BaseModel

app = FastAPI()


class Item(BaseModel):
    id: int
    name: str
    price: float
    is_offer: bool = False


def processed_header(authorization: str = Header("")) -> str:
    return authorization.split(" ").pop(0)


@app.get("/items/{id}")
def read_root(id: int, response: Response, *, auth: str = Depends(processed_header)):
    response.headers["x-authinfo"] = auth
    return Item(id=id, name="Hello", price=1.25, is_offer=False)
