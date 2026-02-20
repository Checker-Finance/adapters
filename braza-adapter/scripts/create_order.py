import os
import hmac
import base64
import hashlib
import time
import requests
from urllib.parse import urlparse

BASE_URL = "https://api-staging.checker.finance"
RFQ_PATH = "/api/v1/request-for-quotes"


def create_signature(nonce: str, path: str, api_key: str, secret_key: str) -> str:
    """
    Create signature with message: 'apiKey:nonce:path'
    """
    message = f"{api_key}:{nonce}:{path}"
    hmac_obj = hmac.new(
        secret_key.encode("utf-8"),
        message.encode("utf-8"),
        hashlib.sha256,
    )
    return base64.b64encode(hmac_obj.digest()).decode("utf-8")


def make_auth_headers(path: str, api_key: str, secret_key: str) -> dict:
    """
    Build headers including x-api-key, x-nonce, x-signature.
    `path` must be the HTTP path portion, e.g. '/api/v1/request-for-quotes'
    """
    nonce = str(int(time.time() * 1000))  # ms since epoch
    signature = create_signature(nonce, path, api_key, secret_key)
    return {
        "x-api-key": api_key,
        "x-nonce": nonce,
        "x-signature": signature,
        "Content-Type": "application/json",
        "Accept": "application/json",
    }


def create_rfq(session: requests.Session, api_key: str, secret_key: str) -> str:
    """
    Create an RFQ and return the RFQ URL returned by the API.
    Adjust the payload as needed.
    """
    url = BASE_URL + RFQ_PATH
    payload = {
        "instrumentPair": "usdc/brl",
        "quantity": 100.00,
        "side": "buy",
        "amountDenomination": "usdc",
        "providers": ["braza"],
    }

    headers = make_auth_headers(RFQ_PATH, api_key, secret_key)
    resp = session.post(url, json=payload, headers=headers, timeout=10)
    resp.raise_for_status()

    # Try to find the RFQ URL from body or Location header
    data = {}
    try:
        data = resp.json()
    except ValueError:
        pass

    rfq_url = (
        data.get("rfqUrl")
        or data.get("rfq_url")
        or data.get("url")
        or resp.headers.get("Location")
    )

    if not rfq_url:
        raise RuntimeError(
            f"Could not determine RFQ URL from response: status={resp.status_code}, body={data}, headers={dict(resp.headers)}"
        )

    # If they returned a relative path, make it absolute
    if rfq_url.startswith("http://"):
            rfq_url = "https://" + rfq_url[len("http://"):]


    print(f"Created RFQ: {rfq_url}")
    return rfq_url


def poll_quotes(
    session: requests.Session,
    rfq_url: str,
    api_key: str,
    secret_key: str,
    max_attempts: int = 10,
    delay_seconds: float = 1.0,
) -> dict:
    """
    Poll {{rfqUrl}}/quotes until at least one quote is returned
    or max_attempts is reached. Returns the chosen quote dict.
    """
    parsed = urlparse(rfq_url)
    quotes_path = parsed.path.rstrip("/") + "/quotes"
    quotes_url = f"{parsed.scheme}://{parsed.netloc}{quotes_path}"

    for attempt in range(1, max_attempts + 1):
        headers = make_auth_headers(quotes_path, api_key, secret_key)
        resp = session.get(quotes_url, headers=headers, timeout=10)
        resp.raise_for_status()
        quotes = resp.json()

        if quotes:
            # You can add logic here to pick the "best" quote.
            # For now we'll just take the first one.
            quote = quotes[0]
            print(f"Received quote on attempt {attempt}: {quote}")
            return quote

        print(f"No quotes yet (attempt {attempt}/{max_attempts}), sleeping...")
        time.sleep(delay_seconds)

    raise TimeoutError("No quotes received within polling limit")


def execute_quote(
    session: requests.Session,
    rfq_url: str,
    quote_id: str,
    api_key: str,
    secret_key: str,
) -> dict:
    """
    POST to {{rfqUrl}}/execute with {"winningQuoteId": quote_id}
    """
    from urllib.parse import urlparse

    parsed = urlparse(rfq_url)
    execute_path = parsed.path.rstrip("/") + "/execute"
    execute_url = f"{parsed.scheme}://{parsed.netloc}{execute_path}"

    payload = {"winningQuoteId": quote_id}
    print(f"sending payload {payload} to {execute_path}")
    headers = make_auth_headers(execute_path, api_key, secret_key)

    resp = session.post(execute_url, json=payload, headers=headers, timeout=10)

    if not resp.ok:
        print("---- EXECUTE ERROR ----")
        print("Status code:", resp.status_code)
        print("URL:", execute_url)
        print("Method actually used:", resp.request.method)
        print("Path used for signature:", execute_path)
        print("Request headers:", {k: v for k, v in resp.request.headers.items()
                                   if k.lower().startswith("x-") or k.lower() in ("content-type", "accept")})
        print("Request body:", resp.request.body)
        print("Response headers:", dict(resp.headers))
        print("Response body:", resp.text)
        print("------------------------")
        resp.raise_for_status()

    data = resp.text
    print(f"Execution response: {data}")
    return data

def main():
    api_key = os.environ.get("CHECKER_API_KEY")
    secret_key = os.environ.get("CHECKER_SECRET_KEY")

    if not api_key or not secret_key:
        raise SystemExit(
            "Please set CHECKER_API_KEY and CHECKER_SECRET_KEY environment variables."
        )

    with requests.Session() as session:
        rfq_url = create_rfq(session, api_key, secret_key)
        quote = poll_quotes(session, rfq_url, api_key, secret_key)
        winning_quote_id = quote["quoteId"]

        try:
            execute_quote(session, rfq_url, winning_quote_id, api_key, secret_key)
        except requests.HTTPError as e:
            # We already printed the error body in execute_quote
            print("HTTPError during execute:", e)
            raise


if __name__ == "__main__":
    main()