import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import api from "@/lib/api";
import Button from "@/components/ui/button";
import { Input, Toggle } from "react-daisyui";
import { handleError } from "@/lib/utils";
import { useBucketContext } from "../context";

type Lifecycle = { days: number | null };

const RetentionSection = () => {
  const { bucketName } = useBucketContext();
  const path = `/buckets/${encodeURIComponent(bucketName)}/lifecycle`;

  const query = useQuery({
    queryKey: ["bucket-lifecycle", bucketName],
    queryFn: () => api.get<Lifecycle>(path),
    enabled: !!bucketName,
  });

  const [enabled, setEnabled] = useState(false);
  const [days, setDays] = useState("");

  useEffect(() => {
    if (!query.data) return;
    setEnabled(!!query.data.days);
    setDays(query.data.days ? String(query.data.days) : "");
  }, [query.data]);

  const save = useMutation({
    mutationFn: (value: number) =>
      api.put<Lifecycle>(path, { body: { days: value } }),
    onSuccess: () => {
      toast.success("Retention updated");
      query.refetch();
    },
    onError: handleError,
  });

  const value = Number(days);
  const valid = !enabled || (Number.isInteger(value) && value > 0);
  const current = query.data?.days ?? null;
  const next = enabled ? value : 0;
  const changed = next !== (current ?? 0);

  return (
    <div className="mt-8 w-full">
      <p className="label label-text py-0">Auto delete (retention)</p>

      <label className="flex flex-row items-center gap-3 mt-2 cursor-pointer">
        <Toggle
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          disabled={query.isLoading}
        />
        <span className="text-sm">Delete objects after a number of days</span>
      </label>

      {query.isError ? (
        <p className="text-sm text-error mt-2">
          {(query.error as Error)?.message || "Cannot load retention rule"}
        </p>
      ) : null}

      {enabled && (
        <div className="mt-4 max-w-xs">
          <p className="label label-text">Days</p>
          <Input
            type="number"
            min={1}
            className="w-full"
            value={days}
            onChange={(e) => setDays(e.target.value)}
          />
          <p className="text-xs opacity-70 mt-1">
            Garage runs expiry about once a day, so files go within a day of
            passing this age.
          </p>
        </div>
      )}

      <div className="mt-4">
        <Button
          color="primary"
          size="sm"
          disabled={!valid || !changed || save.isPending || query.isLoading}
          onClick={() => save.mutate(next)}
        >
          {save.isPending ? "Saving..." : "Save retention"}
        </Button>
      </div>
    </div>
  );
};

export default RetentionSection;
