package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func runCompletion(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: shell type required for completion (e.g. 'bash', 'zsh', 'fish')")
		fmt.Fprintln(os.Stderr, "Usage: aws-cost-report completion [bash|zsh|fish]")
		os.Exit(1)
	}

	shell := strings.ToLower(args[0])
	switch shell {
	case "bash":
		generateBashCompletion(os.Stdout)
	case "zsh":
		generateZshCompletion(os.Stdout)
	case "fish":
		generateFishCompletion(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported shell %q (supported: bash, zsh, fish)\n", shell)
		os.Exit(1)
	}
}

func generateBashCompletion(w io.Writer) {
	script := `# bash completion for aws-cost-report
_aws_cost_report() {
    local cur prev words cword
    _init_completion || return

    local commands="generate services server completion version help"
    local flags="--compare --mtd --current-month --daily --granularity --forecast --monthly --trend --format --months --start --end --metrics --group-by --name --account-name --filter --service --tag --tag-filter --exclude-credits --charge-type --record-type --threshold --fail-on-threshold --top --min-cost --slack-webhook --teams-webhook --s3-bucket --s3-key --currency --no-color --mock --config -c --output -o --json-summary --write-summary --version -v --help -h"

    case "${prev}" in
        --charge-type|--record-type)
            COMPREPLY=( $(compgen -W "Usage Credit Refund Tax Upfront Recurring Support Other" -- "${cur}") )
            return 0
            ;;
        --granularity)
            COMPREPLY=( $(compgen -W "MONTHLY DAILY" -- "${cur}") )
            return 0
            ;;
        --format)
            COMPREPLY=( $(compgen -W "table markdown json csv prometheus" -- "${cur}") )
            return 0
            ;;
        --group-by)
            COMPREPLY=( $(compgen -W "SERVICE LINKED_ACCOUNT TAG:" -- "${cur}") )
            return 0
            ;;
        --currency)
            COMPREPLY=( $(compgen -W "USD EUR GBP JPY AUD CAD CHF" -- "${cur}") )
            return 0
            ;;
        --config|-c|--output|-o|--json-summary)
            _filedir
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            return 0
            ;;
    esac

    if [[ ${cword} -eq 1 ]]; then
        if [[ "${cur}" == -* ]]; then
            COMPREPLY=( $(compgen -W "${flags}" -- "${cur}") )
        else
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
        fi
        return 0
    fi

    COMPREPLY=( $(compgen -W "${flags}" -- "${cur}") )
    return 0
}
complete -F _aws_cost_report aws-cost-report
`
	_, _ = io.WriteString(w, script)
}

func generateZshCompletion(w io.Writer) {
	script := `#compdef aws-cost-report

_aws_cost_report() {
    local -a commands
    commands=(
        'generate:Generate monthly billing and usage cost report'
        'services:List official AWS Cost Explorer service names'
        'server:Run as an HTTP service daemon exposing /healthz, /readyz, /report, and /metrics'
        'completion:Generate shell autocompletion script (bash, zsh, fish)'
        'version:Print application version metadata and exit'
        'help:Display command-line usage and help information'
    )

    _arguments -C \
        '1: :->command' \
        '*: :->args'

    case $state in
        command)
            _describe -t commands 'aws-cost-report command' commands
            ;;
        args)
            case $words[1] in
                completion)
                    _values 'shell' 'bash' 'zsh' 'fish'
                    ;;
                *)
                    _arguments \
                        '--compare[Compare Month-over-Month with preceding billing period]' \
                        '--mtd[Query Month-to-Date spend from the 1st of current month up to today]' \
                        '--current-month[Alias for --mtd]' \
                        '--daily[Query daily spend granularity over the billing period]' \
                        '--granularity[Spend granularity]:granularity:(MONTHLY DAILY)' \
                        '--forecast[Query AWS Cost Explorer projected month-end spend forecast and guardrail]' \
                        '--monthly[Display multi-month historical breakdown with each month as a separate column]' \
                        '--trend[Alias for --monthly]' \
                        '--format[Output format]:format:(table markdown json csv prometheus)' \
                        '--months[Number of historical complete months back to query]:months:' \
                        '--start[Explicit billing start date (inclusive)]:date:' \
                        '--end[Explicit billing end date (inclusive)]:date:' \
                        '--group-by[Grouping dimension]:dimension:(SERVICE LINKED_ACCOUNT TAG\:)' \
                        '--name[Human-readable name or alias for the AWS account]:name:' \
                        '--account-name[Alias for --name]:name:' \
                        '--filter[Filter services by name, alias, or substring]:services:' \
                        '--service[Alias for --filter]:services:' \
                        '--tag[Filter costs by AWS Cost Allocation Tag]:tag:' \
                        '--tag-filter[Alias for --tag]:tag:' \
                        '--exclude-credits[Exclude credits and refunds from billing calculations]' \
                        '--charge-type[Filter costs by charge/record type]:charge:(Usage Credit Refund Tax Upfront Recurring Support Other)' \
                        '--record-type[Alias for --charge-type]:charge:(Usage Credit Refund Tax Upfront Recurring Support Other)' \
                        '--threshold[SRE budget alert dollar threshold]:amount:' \
                        '--top[Show top N highest cost items and aggregate remainder]:count:' \
                        '--min-cost[Minimum cost floor to display]:cost:' \
                        '--output[Write formatted report output to specified file]:file:_files' \
                        '-o[Write formatted report output to specified file]:file:_files' \
                        '--json-summary[Write machine-readable summary metrics to JSON file]:file:_files' \
                        '--slack-webhook[Incoming Slack webhook URL]:url:' \
                        '--teams-webhook[Incoming Microsoft Teams webhook URL]:url:' \
                        '--s3-bucket[S3 bucket name to upload cost report]:bucket:' \
                        '--s3-key[S3 object key for uploaded report]:key:' \
                        '--currency[Override currency code and symbol]:currency:(USD EUR GBP JPY AUD CAD CHF)' \
                        '--no-color[Disable ANSI colors in terminal tables]' \
                        '--mock[Run in offline mode with simulated AWS billing data]' \
                        '--config[Path to YAML configuration file]:file:_files' \
                        '-c[Path to YAML configuration file]:file:_files' \
                        '--version[Print version metadata]' \
                        '-v[Print version metadata]' \
                        '--help[Display help information]' \
                        '-h[Display help information]'
                    ;;
            esac
            ;;
    esac
}

_aws_cost_report "$@"
`
	_, _ = io.WriteString(w, script)
}

func generateFishCompletion(w io.Writer) {
	script := `# fish completion for aws-cost-report

# Subcommands
complete -c aws-cost-report -n "__fish_use_subcommand" -a generate -d "Generate monthly billing and usage cost report"
complete -c aws-cost-report -n "__fish_use_subcommand" -a services -d "List official AWS Cost Explorer service names"
complete -c aws-cost-report -n "__fish_use_subcommand" -a server -d "Run as an HTTP service daemon exposing /healthz, /readyz, /report, and /metrics"
complete -c aws-cost-report -n "__fish_use_subcommand" -a completion -d "Generate shell completion script (bash, zsh, fish)"
complete -c aws-cost-report -n "__fish_use_subcommand" -a version -d "Print application version metadata and exit"
complete -c aws-cost-report -n "__fish_use_subcommand" -a help -d "Display command-line usage and help information"

# Subcommand: completion
complete -c aws-cost-report -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"

# Flags
complete -c aws-cost-report -l compare -d "Compare Month-over-Month with preceding billing period"
complete -c aws-cost-report -l mtd -d "Query Month-to-Date spend up to today"
complete -c aws-cost-report -l current-month -d "Alias for --mtd"
complete -c aws-cost-report -l daily -d "Query daily spend granularity over the billing period"
complete -c aws-cost-report -l granularity -x -a "MONTHLY DAILY" -d "Spend granularity"
complete -c aws-cost-report -l forecast -d "Query AWS Cost Explorer projected month-end spend forecast and guardrail"
complete -c aws-cost-report -l monthly -d "Display multi-month historical breakdown in column format"
complete -c aws-cost-report -l trend -d "Alias for --monthly"
complete -c aws-cost-report -l format -x -a "table markdown json csv prometheus" -d "Output format"
complete -c aws-cost-report -l group-by -x -a "SERVICE LINKED_ACCOUNT" -d "Grouping dimension"
complete -c aws-cost-report -l currency -x -a "USD EUR GBP JPY AUD CAD CHF" -d "Currency code"
complete -c aws-cost-report -l exclude-credits -d "Exclude credits and refunds from billing calculations"
complete -c aws-cost-report -l charge-type -x -a "Usage Credit Refund Tax Upfront Recurring Support Other" -d "Filter costs by charge/record type"
complete -c aws-cost-report -l record-type -x -a "Usage Credit Refund Tax Upfront Recurring Support Other" -d "Alias for --charge-type"
complete -c aws-cost-report -l mock -d "Run in offline mode with simulated AWS billing data"
complete -c aws-cost-report -l no-color -d "Disable ANSI color output in terminal tables"
complete -c aws-cost-report -l config -s c -r -d "Path to YAML configuration file"
complete -c aws-cost-report -l output -s o -r -d "Write report output to specified file"
complete -c aws-cost-report -l json-summary -r -d "Write summary JSON to file"
complete -c aws-cost-report -l s3-bucket -d "S3 bucket to upload cost report"
complete -c aws-cost-report -l s3-key -d "S3 object key for uploaded report"
complete -c aws-cost-report -l version -s v -d "Print version information and exit"
complete -c aws-cost-report -l help -s h -d "Display help information"
`
	_, _ = io.WriteString(w, script)
}
