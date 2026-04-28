import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

interface LogModelCellProps {
  modelName: string;
  originModelName?: string;
  targetLabel: string;
  originLabel: string;
}

const formatModelName = (value?: string) => value?.trim() || '—';

export function LogModelCell({ modelName, originModelName, targetLabel, originLabel }: LogModelCellProps) {
  const billedModelName = formatModelName(modelName);
  const requestedModelName = formatModelName(originModelName);
  const trimmedOrigin = originModelName?.trim() ?? '';
  const trimmedTarget = modelName?.trim() ?? '';

  if (!trimmedOrigin || trimmedOrigin === trimmedTarget) {
    return <span className="font-medium">{billedModelName}</span>;
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="font-medium cursor-help underline decoration-dotted underline-offset-4">{billedModelName}</span>
        </TooltipTrigger>
        <TooltipContent align="start">
          <div className="flex flex-col gap-1 text-xs">
            <div>
              <span className="font-medium">{targetLabel}:</span> {billedModelName}
            </div>
            <div>
              <span className="font-medium">{originLabel}:</span> {requestedModelName}
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
