import { PermissionRulesSettings } from './PermissionRulesSettings';
import { AgentTimeoutSettings } from './AgentTimeoutSettings';
import { MCPServersSettings } from './MCPServersSettings';
import { ExecRulesSettings } from './ExecRulesSettings';

export function RuntimeConfigSettings() {
  return (
    <div className="space-y-6">
      <PermissionRulesSettings />
      <ExecRulesSettings />
      <AgentTimeoutSettings />
      <MCPServersSettings />
    </div>
  );
}
