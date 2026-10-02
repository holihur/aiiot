import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

const WRITE = ["owner", "admin", "member"];
const ADMIN = ["owner", "admin"];

// useProjectRole resolves the current user's role in a project from the cached
// project list (which includes myRole) and exposes capability flags, so pages
// can disable or hide actions instead of failing with a 403.
export function useProjectRole(projectId: number) {
  const { data } = useQuery({ queryKey: ["projects"], queryFn: api.listProjects });
  const role = data?.find((p) => p.id === projectId)?.myRole ?? "";
  return {
    role,
    canWrite: WRITE.includes(role),
    canAdmin: ADMIN.includes(role),
  };
}
