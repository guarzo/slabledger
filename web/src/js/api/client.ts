/**
 * API Client core for SlabLedger
 * Handles all HTTP requests to the backend API with retry logic
 */

/**
 * Custom error class for API errors with proper type safety
 */
export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string,
    public data?: {
      error?: string;
      message?: string;
      details?: unknown;
    },
  ) {
    super(message);
    this.name = 'APIError';
    // Maintains proper prototype chain for instanceof checks
    Object.setPrototypeOf(this, APIError.prototype);
  }
}

/**
 * Type guard to check if an error is an APIError
 */
export function isAPIError(error: unknown): error is APIError {
  return error instanceof APIError;
}

/** Default request timeout in milliseconds (30 seconds) */
export const DEFAULT_TIMEOUT_MS = 30000;

/** Timeout for file upload requests in milliseconds (5 minutes) */
export const UPLOAD_TIMEOUT_MS = 300_000;

/**
 * Options for API requests supporting timeout and cancellation
 */
export interface APIRequestOptions {
  /** AbortSignal for request cancellation */
  signal?: AbortSignal;
  /** Request timeout in milliseconds (default: 30000) */
  timeoutMs?: number;
}

export class APIClient {
  baseURL: string;
  maxRetries: number;
  retryDelay: number;
  defaultTimeoutMs: number;

  constructor(baseURL = '/api') {
    this.baseURL = baseURL;
    this.maxRetries = 3;
    this.retryDelay = 1000; // Initial delay in ms
    this.defaultTimeoutMs = DEFAULT_TIMEOUT_MS;
  }

  /**
   * Sleep utility for retry delays
   */
  sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  /**
   * Create an AbortController with automatic timeout.
   * Returns the controller and a cleanup function.
   */
  createTimeoutController(
    timeoutMs: number,
    externalSignal?: AbortSignal
  ): { controller: AbortController; cleanup: () => void } {
    const controller = new AbortController();

    if (externalSignal?.aborted) {
      controller.abort(externalSignal.reason);
      return { controller, cleanup: () => {} };
    }

    const timeoutId = setTimeout(() => controller.abort(new Error('Request timeout')), timeoutMs);

    const handleExternalAbort = () => {
      controller.abort(externalSignal?.reason);
    };
    externalSignal?.addEventListener('abort', handleExternalAbort);

    const cleanup = () => {
      clearTimeout(timeoutId);
      externalSignal?.removeEventListener('abort', handleExternalAbort);
    };

    return { controller, cleanup };
  }

  /**
   * Check if error is retryable (network errors, 5xx, rate limits)
   */
  isRetryableError(status?: number): boolean {
    return !status || status === 429 || status >= 500;
  }

  /**
   * Perform HTTP request with retry logic, timeout, and cancellation support
   * @param url - The URL to fetch
   * @param options - Standard fetch RequestInit options
   * @param attempt - Current retry attempt number
   * @param requestOptions - Additional options for timeout and cancellation
   */
  async fetchWithRetry(
    url: string,
    options: RequestInit = {},
    attempt = 1,
    requestOptions?: { signal?: AbortSignal; timeoutMs?: number }
  ): Promise<Response> {
    const timeoutMs = requestOptions?.timeoutMs ?? this.defaultTimeoutMs;
    const { controller, cleanup } = this.createTimeoutController(timeoutMs, requestOptions?.signal);

    try {
      const response = await fetch(url, { ...options, signal: controller.signal, credentials: 'include' });
      cleanup();

      if (!response.ok) {
        // Try to parse JSON error response
        let data: { error?: string; message?: string; details?: unknown; code?: string } = { error: response.statusText };
        try {
          data = await response.json();
        } catch {
          // Keep default data value
        }

        const error = new APIError(
          data.error || data.message || `API error: ${response.status} ${response.statusText}`,
          response.status,
          data.code,
          data,
        );

        // Retry for retryable errors
        if (this.isRetryableError(response.status) && attempt < this.maxRetries) {
          const delay = this.retryDelay * Math.pow(2, attempt - 1); // Exponential backoff
          await this.sleep(delay);
          return this.fetchWithRetry(url, options, attempt + 1, requestOptions);
        }

        throw error;
      }

      return response;
    } catch (err) {
      cleanup();

      // Handle abort/timeout errors - don't retry these
      if (err instanceof Error && err.name === 'AbortError') {
        // Check if it was a timeout by inspecting the signal reason
        // The AbortError message is unreliable; the reason we passed to abort() is authoritative
        const reason = controller?.signal?.reason;
        const isTimeout = reason instanceof Error && reason.message === 'Request timeout';
        throw new APIError(
          isTimeout ? `Request timed out after ${timeoutMs}ms` : 'Request was cancelled',
          0,
          isTimeout ? 'TIMEOUT' : 'CANCELLED'
        );
      }

      // Network errors (no response) - check if it's our APIError or a native error
      if (isAPIError(err)) {
        throw err; // Re-throw APIError as-is
      }
      // Handle network errors (TypeError from fetch, etc.)
      if (attempt < this.maxRetries) {
        const delay = this.retryDelay * Math.pow(2, attempt - 1);
        await this.sleep(delay);
        return this.fetchWithRetry(url, options, attempt + 1, requestOptions);
      }
      // Convert native errors to APIError for consistent error handling
      const message = err instanceof Error ? err.message : 'Network error';
      throw new APIError(message, 0, 'NETWORK_ERROR');
    }
  }

  /**
   * GET request with optional timeout and cancellation support
   */
  async get<T>(endpoint: string, options?: APIRequestOptions): Promise<T> {
    const response = await this.fetchWithRetry(`${this.baseURL}${endpoint}`, {}, 1, options);
    return response.json();
  }

  /**
   * POST request with optional timeout and cancellation support
   */
  async post<T>(endpoint: string, data?: unknown, options?: APIRequestOptions): Promise<T> {
    const response = await this.fetchWithRetry(
      `${this.baseURL}${endpoint}`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(data),
      },
      1,
      options
    );
    return response.json();
  }

  /**
   * PUT request with optional timeout and cancellation support
   */
  async put<T>(endpoint: string, data?: unknown, options?: APIRequestOptions): Promise<T> {
    const response = await this.fetchWithRetry(
      `${this.baseURL}${endpoint}`,
      {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(data),
      },
      1,
      options
    );
    return response.json();
  }

  /**
   * Delete resource - returns void for empty responses
   * Supports optional JSON body, timeout, and cancellation
   */
  async deleteResource(endpoint: string, opts?: { body?: unknown } & APIRequestOptions): Promise<void> {
    const fetchOptions: RequestInit = { method: 'DELETE' };
    if (opts?.body !== undefined) {
      fetchOptions.headers = { 'Content-Type': 'application/json' };
      fetchOptions.body = JSON.stringify(opts.body);
    }
    const response = await this.fetchWithRetry(
      `${this.baseURL}${endpoint}`,
      fetchOptions,
      1,
      opts
    );
    // Handle 204 No Content - no response body expected
    if (response.status === 204) {
      return;
    }
    // Some APIs return a body on DELETE - consume but don't return
    const text = await response.text();
    // If there's an error in the response body, we should handle it
    if (text && text.trim() !== '') {
      try {
        const data = JSON.parse(text);
        if (data.error) {
          throw new APIError(
            data.error || data.message || `API error: ${response.status} ${response.statusText}`,
            response.status,
            data.code,
            data,
          );
        }
      } catch (e) {
        // If it's our APIError, rethrow it
        if (isAPIError(e)) {
          throw e;
        }
        // Otherwise ignore parse errors - the response was successful
      }
    }
  }

  /**
   * Handle a response that may be 204 No Content or contain a JSON error body.
   * Used by mutation endpoints that return no body on success.
   */
  async expectNoContent(response: Response): Promise<void> {
    if (response.status === 204) return;
    const text = await response.text();
    try {
      if (text) {
        const data = JSON.parse(text);
        if (data.error) throw new APIError(data.error, response.status);
      }
    } catch (e) {
      if (isAPIError(e)) throw e;
      // JSON parse failed on non-204 response
      throw new APIError(text || `Unexpected status ${response.status}`, response.status);
    }
  }

  /**
   * Upload a file to the given endpoint with timeout support.
   * Shared helper used by all file upload methods.
   */
  async uploadFile<T>(endpoint: string, file: File): Promise<T> {
    const formData = new FormData();
    formData.append('file', file);
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), UPLOAD_TIMEOUT_MS);
    try {
      const response = await fetch(`${this.baseURL}${endpoint}`, {
        method: 'POST',
        body: formData,
        credentials: 'include',
        signal: controller.signal,
      });
      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new APIError(
          data.error || data.message || `Upload failed: ${response.status} ${response.statusText}`,
          response.status,
          data.code,
          data,
        );
      }
      return response.json();
    } finally {
      clearTimeout(timeoutId);
    }
  }
}
