import { Info } from 'lucide-react';
import { FormLabel } from '@/components/ui/form';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

export const LabelWithHelp = ({ label, help }: { label: string; help: string }) => (
  <div className="flex items-center gap-1">
    <FormLabel>{label}</FormLabel>
    <Tooltip>
      <TooltipTrigger asChild>
        <Info className="h-4 w-4 text-muted-foreground cursor-help" aria-label={`Help: ${label}`} />
      </TooltipTrigger>
      <TooltipContent className="max-w-xs whitespace-pre-line">{help}</TooltipContent>
    </Tooltip>
  </div>
);
