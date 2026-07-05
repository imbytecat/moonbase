// Browser → S3 direct upload: PUT the file against a presigned URL issued by
// a storage.v1 RPC. The server never proxies bytes.
export async function uploadToPresignedUrl(url: string, file: File): Promise<void> {
  const res = await fetch(url, {
    method: 'PUT',
    headers: { 'Content-Type': file.type },
    body: file,
  })
  if (!res.ok) throw new Error(`upload failed with ${res.status}`)
}
