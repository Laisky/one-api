# Getting started with cross-region inference in Amazon Bedrock | AWS Machine Learning Blog

URL: https://aws.amazon.com/blogs/machine-learning/getting-started-with-cross-region-inference-in-amazon-bedrock/

With the advent of [generative AI solutions](https://aws.amazon.com/generative-ai/), a paradigm shift is underway across industries, driven by organizations embracing foundation models to unlock unprecedented opportunities. [Amazon Bedrock](https://aws.amazon.com/bedrock/) has emerged as the preferred choice for numerous customers seeking to innovate and launch generative AI applications, leading to an exponential surge in demand for model inference capabilities. Bedrock customers aim to scale their worldwide applications to accommodate growth, and require additional burst capacity to handle unexpected surges in traffic. Currently, users might have to engineer their applications to handle scenarios involving traffic spikes that can use service quotas from multiple regions by implementing complex techniques such as client-side load balancing between AWS regions, where Amazon Bedrock service is supported. However, this dynamic nature of demand is difficult to predict, increases operational overhead, introduces potential points of failure, and might hinder businesses from achieving continuous service availability.

**Today**, we are happy to announce the general availability of **_cross-region inference,_** a powerful feature allowing automatic cross-region inference routing for requests coming to Amazon Bedrock. This offers developers using on-demand inference mode, a seamless solution for getting higher throughput and performance, while managing incoming traffic spikes of applications powered by Amazon Bedrock. By opting in, developers no longer have to spend time and effort predicting demand fluctuations. Instead, cross-region inference dynamically routes traffic across multiple regions. Moreover, this capability prioritizes the connected Amazon Bedrock API source region when possible, helping to minimize latency and improve responsiveness. As a result, customers can enhance their applications’ reliability, performance, and efficiency.

Let us dig deeper into this feature where we will cover:

- Key features and benefits of cross-region inference
- Getting started with cross-region inference
- Code samples for defining and leveraging this feature
- How to think about migrating to cross-region inference
- Key considerations
- Best Practices to follow for this feature
- Conclusion

Let’s dig in!

## Key features and benefits.

One of the critical requirements from our customers is the ability to manage bursts and spiky traffic patterns across a variety of generative AI workloads and disparate request shapes. Some of the key features of cross-region inference include:

- Utilize capacity from multiple AWS regions allowing generative AI workloads to scale with demand.
- Compatibility with existing Amazon Bedrock API
- No additional routing or data transfer cost and you pay the same price per token for models as in your source region (the region you made the request to).
- Ability to choose from a range of pre-configured AWS region sets tailored to your needs.

The below image would help to understand how this feature works. Amazon Bedrock makes real-time decisions for every request made via cross-region inference at any point of time. When a request arrives to Amazon Bedrock, a capacity check is performed in the same region where the request originated from, if there is enough capacity the request is fulfilled else a second check determines the region which has capacity to take the request, it is then re-routed to that decided region and results are retrieved for customer request. This ability to perform capacity checks was not available to customers so they had to implement manual checks of every region of choice after receiving an error and then re-route. Further the typical custom implementation of re-routing might be based on round robin mechanism with no insights into the available capacity of a region. With this new capability, Amazon Bedrock takes into account all the aspects of traffic and capacity in real-time, to make the decision on behalf of customers in a fully-managed manner without any extra costs.

![](https://d2908q01vomqb2.cloudfront.net/f1f836cb4ea6efb2a0b1b99f41ad8b103eff4b59/2024/08/26/ML-17303-image001.gif)

**Few points to be aware of:**

1. AWS network backbone is used for data transfer between regions instead of internet or VPC peering, resulting in secure and reliable execution.
2. You can access a select list of models via cross-region inference, which are essentially region agnostic models made available across the entire region-set.
3. You can use this feature in the Amazon Bedrock model invocation APIs (`InvokeModel` and `Converse` API).
4. You can choose whether to use Foundation Models directly via their respective model identifier or use the model via the cross-region inference mechanism. Any inferences performed via this feature will consider on-demand capacity from all of its pre-configured regions to maximize application throughput and performance.
5. There will be additional latency incurred when re-routing happens and, in our testing, it has been a double-digit milliseconds latency add.
6. All terms applicable to the use of a particular model, including any end user license agreement, still apply when using cross-region inference.
7. When using this feature, your **_throughput can reach up to double the default in-region quotas_** in the region that the inference profile is in. The increase in throughput only applies to invocation performed via inference profiles, the regular quota still applies if you opt for in-region model invocation request. To see quotas for on-demand throughput, refer to the **Runtime quotas** section in [Quotas for Amazon Bedrock](https://docs.aws.amazon.com/bedrock/latest/userguide/quotas.html) or use the Service Quotas console.

Let us dive deep into a few important aspects:

1. As part of this launch, you can select either a US Model or EU Model, each of which will include 2-3 preset regions from these geographical locations.
2. **Which models are included?** As part of this launch, we will have Claude 3 family of models (Haiku, Sonnet, Opus) and Claude 3.5 Sonnet made available.
3. **Can we use PrivateLink?** Yes, you will be able to leverage your private links and ensure traffic flows via your VPC with this feature.
4. **Can we use Provisioned Throughput with this feature as well?** Currently, this feature will not apply to Provisioned Throughput and can be used for on-demand inference only.
5. **Where would the logs be for cross-region inference?** The logs and invocations will still be in the region and account where the request originates from. Amazon Bedrock will output indicators on the logs which will show which region actually serviced the request.

Here is an example of the traffic patterns can be from below (map not to scale).

![](https://d2908q01vomqb2.cloudfront.net/f1f836cb4ea6efb2a0b1b99f41ad8b103eff4b59/2024/08/26/ML-17303-image002.jpg)

![](https://d2908q01vomqb2.cloudfront.net/f1f836cb4ea6efb2a0b1b99f41ad8b103eff4b59/2024/08/26/ML-17303-image004.jpg)

A customer with a workload in `eu-west-1` (Ireland) can utilize capacity from both `eu-west-3` (Paris) and `eu-central-1` (Frankfurt), or a workload in `us-east-1` (Northern Virginia) can utilize capacity from `us-west-2` (Oregon), or vice versa. This would keep all inference traffic within the EU or US, respectively.

## Security and Architecture of how cross-region inference looks like

The following diagram shows the high-level architecture for a cross-region inference request:

![](https://d2908q01vomqb2.cloudfront.net/f1f836cb4ea6efb2a0b1b99f41ad8b103eff4b59/2024/08/26/ML-17303-image006.jpg)

The operational flow starts with an Inference request coming to a region for an on-demand baseline model. Capacity evaluations are made on the pre-configured region set. If the request draws capacity from Frankfurt, it will use the AWS Backbone network, ensuring that all traffic remains within the AWS network. The request bypasses the standard API entry-point for the Amazon Bedrock service and goes directly to the Runtime inference service, where the response is returned back to the connected region over the AWS Backbone and then returned to the caller as per a normal inference request. If processing in the chosen region fails for any reason, then another region from the set is tried, `eu-west-1` (Ireland) in this example, followed by `eu-west-3` (Paris), until all pre-configured regions have been attempted. If no region in the region list can handle the inference request, then the API will return the standard “throttled” response.

### Networking and data logging

The AWS-to-AWS traffic flows, such as Region-to-Region (inclusive of Edge Locations and Direct Connect paths), will always traverse AWS-owned and operated backbone paths. This not only reduces threats, such as common exploits and DDoS attacks, but also ensures that all internal AWS-to-AWS traffic uses only trusted network paths. This is combined with inter-Region and intra-Region path encryption and routing policy enforcement mechanisms, all of which use AWS secure facilities. This combination of enforcement mechanisms helps ensure that AWS-to-AWS traffic will never use non-encrypted or untrusted paths, such as the internet, and hence as a result all cross-region inference requests will remain on the AWS backbone at all times.

Log entries will continue to be made in the original source region for both Amazon CloudWatch and AWS CloudTrail, and there will be no additional logs in the re-routed region. In order to indicate that re-routing happened, the related entry in [AWS CloudTrail](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-events.html) will also include the following additional data. The event will contain an **additionalEventData** element, with an **inferenceRegion** key that specifies the region in which your request was processed. If a request was processed without being re-routed, then the **additionalEventData** will be absent from the event

```
{
    "eventVersion": "1.09",
    ...
    "eventSource": "bedrock.amazonaws.com",
    "eventName": "Converse",
    "awsRegion": "us-east-1",
    ...
    "additionalEventData": {
        "inferenceRegion": "us-west-2"
    },
    ...
}
```

The same information is also available in the [Amazon Bedrock Model Invocation Log](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html). This log needs to be enabled first, with logging destination either into Amazon CloudWatch logs or an Amazon S3 bucket:

```
{
    "schemaType": "ModelInvocationLog",
    "schemaVersion": "1.0",
    ...
    "region": "us-east-1",
    "operation": "Converse",
    ...
    "inferenceRegion": "us-west-2"
}
```

With Amazon CloudWatch Logs, you can create [metrics](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/working_with_metrics.html) about the performance of your application. Using the **inferenceRegion** key from either the CloudTrail event or the Amazon Bedrock model invocation log, you can augment your dashboards and monitoring system to differentiate between Amazon Bedrock requests processed in the source region vs. re-routed ones. The code sample for this is available on this [GitHub repository](https://github.com/aws-samples/amazon-bedrock-workshop/blob/main/07_Cross_Region_Inference/Getting_started_with_Cross-region_Inference.ipynb) under “**Monitoring, Logging, and Metrics**” and it will create a suitable metric using the model invocation log which you can view in your CloudWatch dashboard.

### Identity and Access Management

[AWS Identity and Access Management](https://docs.aws.amazon.com/IAM/latest/UserGuide/introduction.html) (IAM) is key to securely managing your identities and access to AWS services and resources. Before you can use cross-region inference, check that your role has access to the cross-region inference API actions. For more details, see [here](https://docs.aws.amazon.com/bedrock/latest/userguide/cross-region-inference-prereq.html).An example policy, which allows the caller to use the cross-region inference with the `InvokeModel*` APIs for any model in the `us-east-1` and `us-west-2` region is as follows:

```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["bedrock:InvokeModel*"],
      "Resource: [
          "arn:aws:bedrock:us-east-1:<account_id>:inference-profile/*",
          "arn:aws:bedrock:us-east-1::foundation-model/*",
          "arn:aws:bedrock:us-west-2::foundation-model/*"
      ]
    }
  ]
}
```

## Getting started with Cross-region inference

To get started with cross-region inference, you make use of **_Inference Profiles_** in Amazon Bedrock. An inference profile for a model, configures different model ARNs from respective AWS regions and abstracts them behind a unified model identifier (both id and ARN). Just by simply using this new inference profile identifier with the `InvokeModel` or `Converse` API, you can use the cross-region inference feature.

For the models available in your region and via cross-region inference you can start using these models via the method below. But you should also request access to the models available only via cross-region inference as well. For example, to gain access to make calls to the Anthropic’s Claude 3 Haiku inference profile from the US West (Oregon) Region, go to the model access page in `us-west-2` Amazon Bedrock console to grant access. Refer to [Prerequisites for cross-region inference](https://docs.aws.amazon.com/bedrock/latest/userguide/cross-region-inference-prereq.html) for more details.

Here are the steps to start using cross-region inference with the help of inference profiles:

1. **List Inference Profiles** You can list the inference profiles available in your region by either signing in to Amazon Bedrock AWS console or API.
   - Console
     1. From the left-hand pane, select “Cross-region Inference”
     2. You can explore different inference profiles available for your region(s).
     3. Copy the inference profile ID and use it in your application, as described in the section below
   - API It is also possible to list the inference profiles available in your region via boto3 SDK or AWS CLI.

     ```
     aws bedrock list-inference-profiles
     ```

You can observe how different inference profiles have been configured for various geo locations comprising of multiple AWS regions. For example, the models with the prefix us. are configured for AWS regions in USA, whereas models with `eu`. are configured with the regions in European Union (EU).

1. **Modify Your Application**
   1. Update your application to use the inference profile ID/ARN from console or from the API response as `modelId` in your requests via `InvokeModel` or `Converse` API.
   2. This new inference profile will automatically manage inference throttling and re-route your request(s) across multiple AWS Regions (as per configuration).
2. **Monitor and Adjust**
   1. Use Amazon CloudWatch to monitor your inference traffic and latency across regions.
   2. Adjust the use of inference profile vs FMs directly based on your observed traffic patterns and performance requirements.

## Code example to leverage Inference Profiles

Use of **_inference profiles_** is similar to that of foundation models in Amazon Bedrock using the `InvokeModel` or `Converse` API, the only difference between the `modelId` is addition of a prefix such as `us`. or `eu`.

### Foundation Model

```
modelId = 'anthropic.claude-3-5-sonnet-20240620-v1:0'
bedrock_runtime.converse(
  modelId=modelId,
  system=[{
    "text": "You are an AI assistant."
  }],
  messages=[{
    "role": "user",
    "content": [{"text": "Tell me about Amazon Bedrock."}]
  }]
)
```

### Inference Profile

```
modelId = 'eu.anthropic.claude-3-5-sonnet-20240620-v1:0'
bedrock_runtime.converse(
  modelId=modelId,
  system=[{
    "text": "You are an AI assistant."
  }],
  messages=[{
    "role": "user",
    "content": [{"text": "Tell me about Amazon Bedrock."}]
  }]
)
```

### Deep Dive

While it is straight forward to start using inference profiles, you first need to know which inference profiles are available as part of your region. Start with the list of inference profiles and observe models available for this feature. This is done through the AWS CLI or SDK.

```
import boto3
bedrock_client = boto3.client("bedrock", region_name="us-east-1")
bedrock_client.list_inference_profiles()
```

You can expect an output similar to the one below:

```
{
  "inferenceProfileSummaries": [
    {
     "inferenceProfileName": "us. Anthropic Claude 3.5 Sonnet",
        "models": [
           {
             "modelArn": "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-5-sonnet-20240620-v1:0"
           },
           {
             "modelArn": "arn:aws:bedrock:us-west-2::foundation-model/anthropic.claude-3-5-sonnet-20240620-v1:0"
           }
        ],
        "description": "Routes requests to Anthropic Claude 3.5 Sonnet in us-east-1 and us-west-2",
        "createdAt": "2024-XX-XXT00:00:00Z",
        "updatedAt": "2024-XX-XXT00:00:00Z",
        "inferenceProfileArn": "arn:aws:bedrock:us-east-1:<account_id>:inference-profile/us.anthropic.claude-3-5-sonnet-20240620-v1:0",
        "inferenceProfileId": "us.anthropic.claude-3-5-sonnet-20240620-v1:0",
        "status": "ACTIVE",
        "type": "SYSTEM_DEFINED"
    },
    ...
  ]
}
```

The difference between ARN for a foundation model available via Amazon Bedrock and the inference profile can be observed as:

**Foundation Model:** `arn:aws:bedrock:us-east-1::**foundation-model**/anthropic.claude-3-5-sonnet-20240620-v1:0`

**Inference Profile:** `arn:aws:bedrock:us-east-1:<account_id>:**inference-profile**/us.anthropic.claude-3-5-sonnet-20240620-v1:0`

Choose the configured inference profile, and start sending inference requests to your model’s endpoint as usual. Amazon Bedrock will automatically route and scale the requests across the configured regions as needed. You can choose to use both ARN as well as ID with the `Converse` API whereas just the inference profile ID with the `InvokeModel` API. It is important to note which models are [supported](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html#conversation-inference-supported-models-features) by `Converse` API.

```
import boto3

source_region ="<source-region-name>" #us-east-1, eu-central-1
bedrock_runtime = boto3.client("bedrock-runtime", region_name= source_region)
inferenceProfileId = '<regional-prefix>.anthropic.claude-3-5-sonnet-20240620-v1:0'

# Example with Converse API
system_prompt = "You are an expert on AWS AI services."
input_message = "Tell me about AI service for Foundation Models"
response = bedrock_runtime.converse(
    modelId = inferenceProfileId,
    system = [{"text": system_prompt}],
    messages=[{
        "role": "user",
        "content": [{"text": input_message}]
    }]
)

print(response['output']['message']['content'])
us-east-1 or eu-central-1
```

In the code sample above you must specify <your-source-region-name> such as US regions including `us-east-1`, `us-west-2` or EU regions including `eu-central-1`, `eu-west-1`, `eu-west-3`. The <regional-prefix> will then be relative, either `us` or `eu`.

Adapting your applications to use Inference Profiles for your Amazon Bedrock FMs is quick and easy with steps above. No significant code changes are required on the client side. Amazon Bedrock handles the cross-region inference transparently. Monitor CloudTrail logs to check if your request is automatically re-routed to another region as described in the section above.

## How to think about adopting to the new cross-region inference feature?

When considering the adoption of this new capability, it’s essential to carefully evaluate your application requirements, traffic patterns, and existing infrastructure. Here’s a step-by-step approach to help you plan and adopt cross-region inference:

1. **Assess your current workload and traffic patterns.** Analyze your existing generative AI workloads and identify those that require significant throughput demand, including peak loads, geographical distribution, and any seasonal or cyclical variations.
2. **Evaluate the potential benefits of cross-region inference.** Consider the potential advantages of leveraging cross-region inference, with significantly improved throughput and better performance for global users. Estimate the potential cost savings by not having to implement a custom logic of your own and pay for data transfer (as well as different token pricing for models) or efficiency gains by off-loading multiple regional deployments into a single, fully-managed distributed solution.
3. **Plan and execute the migration.** Update your application code to use the inference profile ID/ARN instead of individual foundation model IDs, following the provided code sample above. Test your application thoroughly in a non-production environment, simulating various traffic patterns and failure scenarios. Monitor your application’s performance, latency, and cost during the migration process, and make adjustments as needed.
4. **Develop new applications with cross-region inference in mind.** For new application development, consider designing with cross-region inference as the foundation, leveraging inference profiles from the start.

## Key Considerations

### Impact on Current Generative AI Workloads

Inference profiles are designed to be compatible with existing Amazon Bedrock APIs, such as `InvokeModel` and `Converse`. Also, any third-party/opensource tool which uses these APIs such as LangChain can be used with inference profiles. This means that you can seamlessly integrate inference profiles into your existing workloads without the need for significant code changes. Simply update your application to use the inference profiles ARN instead of individual model IDs, and Amazon Bedrock will handle the cross-region routing transparently.

### Impact on Pricing

The feature comes with no additional cost to you. You pay the same price per token of individual models in your source region. There is no additional cost associated with cross-region inference including the failover capabilities provided by this feature. This includes management, data-transfer, encryption, network usage and potential differences in price per million token per model.

### Regulations, Compliance, and Data Residency

Although none of the customer data is stored in any region when using cross-region inference, it’s important to consider that your inference data can be processed and transmitted across multiple pre-configured regions as defined in the inference profile. If you have strict data residency or compliance requirements, you should carefully evaluate whether cross-region inference aligns with your policies and regulations.

## Conclusion

In this blog we introduced the latest feature from Amazon Bedrock, cross-region inference via inference profiles, and a peek into how it operates and also dived into some of the how-to’s and points for considerations. The code sample for this feature is available on this [GitHub repository](https://github.com/aws-samples/amazon-bedrock-workshop/blob/main/07_Cross_Region_Inference/Getting_started_with_Cross-region_Inference.ipynb). This feature empowers developers to increase the throughput and performance of their applications, without the need to spend time and effort building complex routing structures. This feature is now generally available in US and EU for supported models.

### About the authors

**Talha Chattha** is a Generative AI Specialist Solutions Architect at Amazon Web Services, based in Stockholm. Talha helps establish practices to ease the path to production for Gen AI workloads. Talha is an expert in Amazon Bedrock and supports customers across entire EMEA. He holds passion about meta-agents, scalable on-demand inference, advanced RAG solutions and cost optimized prompt engineering with LLMs. When not shaping the future of AI, he explores the scenic European landscapes and delicious cuisines. Connect with Talha at LinkedIn using /in/talha-chattha/.

[**Rupinder Grewal**](https://www.linkedin.com/in/rupinder-grewal-ml/) is a Senior AI/ML Specialist Solutions Architect with AWS. He currently focuses on the serving of models and MLOps on Amazon SageMaker. Prior to this role, he worked as a Machine Learning Engineer building and hosting models. Outside of work, he enjoys playing tennis and biking on mountain trails.

**Sumit Kumar** is a Principal Product Manager, Technical at AWS Bedrock team, based in Seattle. He has 12+ years of product management experience across a variety of domains and is passionate about AI/ML. Outside of work, Sumit loves to travel and enjoys playing cricket and Lawn-Tennis.

**Dr. Andrew Kane** is an AWS Principal WW Tech Lead (AI Language Services) based out of London. He focuses on the AWS Language and Vision AI services, helping our customers architect multiple AI services into a single use-case driven solution. Before joining AWS at the beginning of 2015, Andrew spent two decades working in the fields of signal processing, financial payments systems, weapons tracking, and editorial and publishing systems. He is a keen karate enthusiast (just one belt away from Black Belt) and is also an avid home-brewer, using automated brewing hardware and other IoT sensors.
