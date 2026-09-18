from fastapi import APIRouter
from fastapi.responses import JSONResponse, Response

from .. import media

router = APIRouter()


@router.get("/files/{name}")
async def get_file(name: str):
    if not media.exists(name):
        return JSONResponse({"error": {"message": f"no such file {name}"}}, status_code=404)
    return Response(media.read(name), media_type=media.media_type(name))
