/** All API response data should follow this format */
interface ApiResponseData<T> {
  code: number
  data: T
  message: string
}
