locals {
  availability_zone = "us-east-1a"
}

data "aws_iam_policy_document" "ec2_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ssm_role" {
  name               = "ec2-ssm-role"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume_role.json
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  role       = aws_iam_role.ssm_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ssm_profile" {
  name = "ec2-ssm-profile"
  role = aws_iam_role.ssm_role.name
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnet" "default_public" {
  vpc_id            = data.aws_vpc.default.id
  availability_zone = local.availability_zone
  default_for_az    = true
}

resource "aws_eip" "nat" {
  domain = "vpc"

  tags = {
    Name = "default-vpc-nat-eip"
  }
}

resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  subnet_id     = data.aws_subnet.default_public.id # Must be a public subnet

  tags = {
    Name = "default-vpc-nat-gw"
  }
}

resource "aws_subnet" "private" {
  vpc_id                  = data.aws_vpc.default.id
  cidr_block              = "172.31.96.0/20"
  availability_zone       = local.availability_zone
  map_public_ip_on_launch = false

  tags = {
    Name = "default-vpc-private-subnet"
  }
}

resource "aws_route_table" "private" {
  vpc_id = data.aws_vpc.default.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main.id
  }

  tags = {
    Name = "default-vpc-private-rt"
  }
}

resource "aws_route_table_association" "private" {
  subnet_id      = aws_subnet.private.id
  route_table_id = aws_route_table.private.id
}

# Latest Amazon Linux 2023 (x86_64)
data "aws_ssm_parameter" "al2023" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}

resource "aws_instance" "app" {
  ami                         = data.aws_ssm_parameter.al2023.value
  instance_type               = "t3.micro"
  iam_instance_profile        = aws_iam_instance_profile.ssm_profile.name
  availability_zone           = "us-east-1a"
  associate_public_ip_address = false
  subnet_id                   = aws_subnet.private.id
  user_data                   = file("${path.module}/bootstrap.sh")
  depends_on                  = [aws_nat_gateway.main]
}
