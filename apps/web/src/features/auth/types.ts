export interface User {
  id: string
  email: string
  name: string
  avatar_url: string | null
  email_verified_at: string | null
  is_admin: boolean
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface UserResponse {
  user: User
}

export interface MessageResponse {
  message: string
}
