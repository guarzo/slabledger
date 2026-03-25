export interface AllowedEmail {
  Email: string;
  AddedBy: number | null;
  CreatedAt: string;
  Notes: string;
}

export interface AdminUser {
  id: number;
  username: string;
  email: string;
  avatar_url: string;
  is_admin: boolean;
  last_login_at: string | null;
}
