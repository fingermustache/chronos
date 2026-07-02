
## Start a PostgreSQL container for the test DB (if you don’t have one)
```
docker run --rm -d \
  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=test_db -p 5432:5432 postgres:15
  ```

## With PIP
```
pip install pytest psycopg2-binary
pytest -q tests/test_basic_schema.py
```

## On MacOS
```
python3 -m pip install pytest psycopg2-binary
python3 -m pytest -q tests/test_basic_schema.py
```